import { STUDIO_STORAGE_NAME } from "../src/lib/studio-identity";
import { expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { runInNewContext } from "node:vm";
import { createCanvasStateWriter } from "../src/lib/canvas/canvas-editor-state";
import * as fileUpload from "../src/lib/canvas/canvas-file-upload";
import { recordReplacedMediaVersion } from "../src/lib/canvas/canvas-media-versions";
import * as resources from "../src/services/api/resources";
import { apiClient } from "../src/services/api/request";
import { getActiveUserScope, setActiveUserScope } from "../src/lib/user-scope";
import { CanvasNodeType, type CanvasNodeData } from "../src/types/canvas";
import type { useCanvasUpload } from "../src/pages/canvas/use-canvas-upload";

// Execute the production hook and resource protocol with an isolated hook scheduler.
// Browser persistence and host/object HTTP responses are the controlled boundaries.
const hookPath = fileURLToPath(new URL("../src/pages/canvas/use-canvas-upload.ts", import.meta.url));
const imports = new Map<string, string[]>();
for (const match of readFileSync(hookPath, "utf8").matchAll(/import \{([^\n]+)\} from "([^"]+)"/g)) {
    imports.set(
        match[2],
        match[1]
            .split(",")
            .map((name) => name.trim())
            .filter((name) => !name.startsWith("type ")),
    );
}
const build = await Bun.build({
    entrypoints: [hookPath],
    target: "browser",
    format: "cjs",
    plugins: [
        {
            name: "upload-hook-boundaries",
            setup(builder) {
                builder.onResolve({ filter: /^(react|nanoid|antd|@)/ }, ({ path }) => ({ path, namespace: "upload-test" }));
                builder.onLoad({ filter: /.*/, namespace: "upload-test" }, ({ path }) => ({ contents: `export const { ${imports.get(path)?.join(", ") || ""} } = dependencies[${JSON.stringify(path)}];`, loader: "js" }));
            },
        },
    ],
});
expect(build.success).toBe(true);
const hookSource = await build.outputs[0].text();
let nextUploadIdentity = 0;

function mount(nodes: CanvasNodeData[] = [], canvasId = "canvas-a", projectId = "project-a") {
    const nodesRef = { current: nodes };
    const snapshots: CanvasNodeData[][] = [];
    const errors: string[] = [];
    const assets: CanvasNodeData[] = [];
    const cleanups: Array<() => void> = [];
    const noop = () => {};
    const dependencies = Object.fromEntries([...imports.keys()].map((path) => [path, {}]));
    Object.assign(dependencies, {
        react: {
            useCallback: (fn: unknown) => fn,
            useMemo: (fn: () => unknown) => fn(),
            useRef: (current: unknown) => ({ current }),
            useState: (value: unknown) => [value, noop],
            useEffect: (fn: () => (() => void) | undefined) => {
                const cleanup = fn();
                if (cleanup) cleanups.push(cleanup);
            },
        },
        nanoid: { nanoid: () => `${canvasId}-${nextUploadIdentity++}` },
        antd: { App: { useApp: () => ({ message: { error: (value: string) => errors.push(value), warning: noop, success: noop } }) } },
        "@tanstack/react-query": { useQueryClient: () => ({ invalidateQueries: async () => {} }) },
        "@/i18n": { translate: (key: string) => key },
        "@/lib/user-scope": { getActiveUserScope },
        "@/lib/canvas/canvas-file-upload": fileUpload,
        "@/lib/canvas/canvas-media-versions": { recordReplacedMediaVersion },
        "@/services/api/resources": resources,
        "@/types/canvas": { CanvasNodeType },
        "@/services/project-asset-sync": {
            ensureCanvasNodeAsset: async ({ node }: { node: CanvasNodeData }) => {
                assets.push(structuredClone(node));
                return { assetId: "business-asset" };
            },
        },
        "@/services/canvas-upload-preview": { registerUploadPreview: noop, releaseUploadPreview: noop },
    });
    const module = { exports: {} as { useCanvasUpload: typeof useCanvasUpload } };
    runInNewContext(hookSource, { module, exports: module.exports, dependencies, window: { setTimeout: () => 1, clearTimeout: noop }, performance, File, DOMException, console });
    const api = module.exports.useCanvasUpload({
        canvasId,
        domainProjectId: projectId,
        nodesRef,
        selectedNodeIdsRef: { current: new Set() },
        getCanvasCenter: () => ({ x: 0, y: 0 }),
        screenToCanvas: () => ({ x: 0, y: 0 }),
        setNodes: createCanvasStateWriter(nodesRef, noop),
        setSelectedNodeIds: noop,
        setSelectedConnectionId: noop,
        setContextMenu: noop,
        setDialogNodeId: noop,
        persistLocalEdits: async () => {
            snapshots.push(structuredClone(nodesRef.current));
        },
    });
    return { api, nodesRef, snapshots, errors, assets, unmount: () => cleanups.forEach((fn) => fn()) };
}

async function fixture(run: (state: { calls: string[]; commands: string[]; puts: number[]; values: Map<string, string>; beforePut?: (part: number) => Promise<void>; beforeCreate?: () => void }) => Promise<void>) {
    const previousWindow = Object.getOwnPropertyDescriptor(globalThis, "window");
    const previousScope = getActiveUserScope();
    const adapter = apiClient.defaults.adapter;
    const previousFetch = globalThis.fetch;
    const state = { calls: [] as string[], commands: [] as string[], puts: [] as number[], values: new Map<string, string>(), beforePut: undefined as ((part: number) => Promise<void>) | undefined, beforeCreate: undefined as (() => void) | undefined };
    const session = {
        id: "local-original",
        upload: {
            protocol: "oceanway.materials.v2",
            upload_id: "upload-original",
            state: "awaiting_upload",
            byte_length: "6",
            part_size: "3",
            tickets: [1, 2].map((part_number) => ({ part_number, method: "PUT", url: `https://objects.example.test/${part_number}`, headers: {}, expires_at: "2099-01-01T00:00:00Z" })),
        },
    };
    try {
        Object.defineProperty(globalThis, "window", { configurable: true, value: { localStorage: { getItem: (key: string) => state.values.get(key) ?? null, setItem: (key: string, value: string) => state.values.set(key, value) } } });
        setActiveUserScope("canvas-upload-owner");
        apiClient.defaults.adapter = async (config) => {
            state.calls.push(`${config.method} ${config.url}`);
            let data: unknown = session;
            if (config.method === "post" && config.url === "/resources/platform-uploads") {
                state.beforeCreate?.();
                state.commands.push(JSON.parse(config.data).commandId);
            }
            if (config.url?.endsWith("/complete")) {
                expect(JSON.parse(config.data).parts).toEqual([
                    { part_number: 1, etag: "etag-1" },
                    { part_number: 2, etag: "etag-2" },
                ]);
                data = { ...session, upload: { ...session.upload, state: "ready" }, resource: { id: `resource-${state.commands.length}`, provider: "platform", status: "ready" } };
            }
            return { config, status: 200, statusText: "OK", headers: {}, data: { code: 0, data } };
        };
        globalThis.fetch = (async (url) => {
            const part = Number(new URL(String(url)).pathname.slice(1));
            state.puts.push(part);
            await state.beforePut?.(part);
            return new Response(null, { status: 200, headers: { ETag: `etag-${part}` } });
        }) as typeof fetch;
        await run(state);
    } finally {
        resources.clearResourceAccessCache();
        setActiveUserScope(previousScope);
        if (previousWindow) Object.defineProperty(globalThis, "window", previousWindow);
        else Reflect.deleteProperty(globalThis, "window");
        apiClient.defaults.adapter = adapter;
        globalThis.fetch = previousFetch;
    }
}

const file = () => new File(["abcdef"], "original.txt", { type: "text/plain" });

test("canvas reopens the persisted upload identity, resumes only the missing part and replaces completed media with a new command", async () => {
    await fixture(async (state) => {
        const first = mount();
        state.beforeCreate = () => expect(first.snapshots[0]?.[0]?.metadata?.fileUploadIdentity?.key).toBeTruthy();
        state.beforePut = async (part) => {
            if (part === 2) throw new TypeError("connection interrupted");
        };
        expect(await first.api.createFileNode(file(), { x: 0, y: 0 })).toBeNull();
        const interrupted = structuredClone(first.nodesRef.current[0]);
        const identity = interrupted.metadata?.fileUploadIdentity;
        expect(interrupted.metadata?.fileUpload).toBe("error");
        first.unmount();
        resources.clearResourceAccessCache();
        const reopened = mount([fileUpload.interruptFileUpload(interrupted)]);
        state.beforeCreate = undefined;
        const callsBeforeMismatch = state.calls.length;
        expect(await reopened.api.replaceNodeMedia(interrupted.id, new File(["ghijkl"], "different.txt", { type: "text/plain" }))).toBe(false);
        expect(state.calls.length).toBe(callsBeforeMismatch);
        expect(reopened.nodesRef.current[0].metadata?.fileUploadIdentity).toEqual(identity);
        state.beforePut = undefined;
        expect(await reopened.api.replaceNodeMedia(interrupted.id, file())).toBe(true);
        expect(state.puts).toEqual([1, 2, 2]);
        expect(state.commands).toHaveLength(1);
        expect(state.calls).toContain("get /resources/platform-uploads/local-original");
        expect(reopened.nodesRef.current[0].id).toBe(interrupted.id);
        expect(reopened.nodesRef.current[0].metadata?.fileUploadIdentity).toBeUndefined();
        expect(reopened.nodesRef.current[0].metadata?.storageKey).toBe("resource:resource-1");
        expect(await reopened.api.replaceNodeMedia(interrupted.id, file())).toBe(true);
        expect(state.commands).toHaveLength(2);
        expect(state.commands[0]).not.toBe(state.commands[1]);
        expect(state.puts).toEqual([1, 2, 2, 1, 2]);
        reopened.unmount();
    });
});

test("pending canvas uploads reject copied project and account contexts before HTTP", async () => {
    await fixture(async (state) => {
        const original = mount();
        state.beforePut = async () => {
            throw new TypeError("interrupted");
        };
        await original.api.createFileNode(file(), { x: 0, y: 0 });
        const node = structuredClone(original.nodesRef.current[0]);
        const count = state.calls.length;
        for (const [canvasId, projectId, owner] of [
            ["canvas-b", "project-a", "canvas-upload-owner"],
            ["canvas-a", "project-b", "canvas-upload-owner"],
            ["canvas-a", "project-a", "another-owner"],
        ]) {
            setActiveUserScope(owner);
            const copy = mount([structuredClone(node)], canvasId, projectId);
            expect(await copy.api.replaceNodeMedia(node.id, file())).toBe(false);
            expect(state.calls).toHaveLength(count);
            expect(copy.nodesRef.current[0].metadata?.fileUploadIdentity).toEqual(node.metadata?.fileUploadIdentity);
            copy.unmount();
        }
        original.unmount();
    });
});

test("late upload acknowledgements cannot complete or mutate a canvas after its account changes", async () => {
    await fixture(async (state) => {
        const view = mount();
        let beforeSwitch: CanvasNodeData[] = [];
        state.beforePut = async () => {
            beforeSwitch = structuredClone(view.nodesRef.current);
            setActiveUserScope("next-owner");
        };
        expect(await view.api.createFileNode(file(), { x: 0, y: 0 })).toBeNull();
        expect(view.nodesRef.current).toEqual(beforeSwitch);
        expect(view.assets).toHaveLength(0);
        expect(state.calls.some((call) => call.endsWith("/complete"))).toBe(false);
        const checkpoints = [...state.values].filter(([key]) => key.startsWith(`${STUDIO_STORAGE_NAME}:studio-material-upload:`));
        expect(checkpoints).toHaveLength(1);
        expect(JSON.parse(checkpoints[0][1]).parts).toEqual([]);
        view.unmount();
    });
});

test("a response after canvas unmount preserves the pending node without adopting its result", async () => {
    await fixture(async (state) => {
        const view = mount();
        let beforeUnmount: CanvasNodeData[] = [];
        state.beforePut = async () => {
            beforeUnmount = structuredClone(view.nodesRef.current);
            view.unmount();
        };
        expect(await view.api.createFileNode(file(), { x: 0, y: 0 })).toBeNull();
        expect(view.nodesRef.current).toEqual(beforeUnmount);
        expect(view.assets).toHaveLength(0);
    });
});
