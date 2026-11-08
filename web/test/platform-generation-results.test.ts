import { afterEach, beforeEach, expect, test } from "bun:test";
import localforage from "localforage";
import { apiClient } from "@/services/api/request";
import { clearResourceAccessCache } from "@/services/api/resources";
import type { GenerationTask } from "@/services/api/task-center";
import { consumeGenerationTaskMessage, consumeGenerationTaskNode, materializeGenerationTaskAssets } from "@/services/project-asset-sync";
import { applyMaterializedGenerationTaskResultToNodes, generationTaskOutputsApplied } from "@/lib/canvas/canvas-generation-task-sync";
import { applyCanvasGenerationTaskNodeEffect, prepareCanvasMediaNodeAsset } from "@/services/canvas-generation-consumer";
import { parseCanvasStorageDocument } from "@/lib/canvas/canvas-storage-revision";
import { localForageStorageForScope } from "@/lib/localforage-storage";
import { useAssetStore } from "@/stores/use-asset-store";
import { useUserStore } from "@/stores/use-user-store";
import { CANVAS_STORE_KEY, flushCanvasStorePersistence, useCanvasStore, withCanvasStorePersistenceSuppressed } from "@/stores/canvas/use-canvas-store";
import { CanvasNodeType, type CanvasNodeData } from "@/types/canvas";

const originalAdapter = apiClient.defaults.adapter;
const originalWindow = Object.getOwnPropertyDescriptor(globalThis, "window");
const originalGet = localforage.getItem;
const originalSet = localforage.setItem;
const originalRemove = localforage.removeItem;
const initialProjects = useCanvasStore.getState().projects;
const initialIdentity = useUserStore.getState().platformIdentity;
let scope = "platform-generation-a";
let requests: string[];
let status = "ready";
let provider = "platform";
let responseCode = 0;
let duringRead: (() => void) | undefined;
let values: Map<string, unknown>;

beforeEach(() => {
    scope = "platform-generation-a";
    requests = [];
    status = "ready";
    provider = "platform";
    responseCode = 0;
    duringRead = undefined;
    values = new Map();
    localforage.getItem = (async (key: string) => values.get(key) ?? null) as typeof originalGet;
    localforage.setItem = (async (key: string, value: unknown) => {
        values.set(key, value);
        return value;
    }) as typeof originalSet;
    localforage.removeItem = (async (key: string) => {
        values.delete(key);
    }) as typeof originalRemove;
    Object.defineProperty(globalThis, "window", { configurable: true, value: { localStorage: { getItem: () => scope }, setTimeout, clearTimeout } });
    clearResourceAccessCache();
    useUserStore.setState({ platformIdentity: true });
    apiClient.defaults.adapter = async (config) => {
        requests.push(config.url || "");
        const match = /^\/resources\/(image-\d|video-1)$/.exec(config.url || "");
        if (config.method !== "get" || !match) throw new Error(`禁止旧资产/外部上传或供应商请求: ${config.method} ${config.url}`);
        duringRead?.();
        const id = match[1];
        return {
            config,
            status: 200,
            statusText: "OK",
            headers: {},
            data: {
                code: responseCode,
                msg: "资源不存在或已失效",
                data: { resource: { id, userId: scope, kind: id.startsWith("image") ? "image" : "video", provider, status, width: 640, height: 360, size: 10, mimeType: id.startsWith("image") ? "image/png" : "video/mp4" } },
            },
        };
    };
});

afterEach(() => {
    apiClient.defaults.adapter = originalAdapter;
    clearResourceAccessCache();
    useUserStore.setState({ platformIdentity: initialIdentity });
    withCanvasStorePersistenceSuppressed(() => useCanvasStore.setState({ projects: initialProjects }));
    localforage.getItem = originalGet;
    localforage.setItem = originalSet;
    localforage.removeItem = originalRemove;
    if (originalWindow) Object.defineProperty(globalThis, "window", originalWindow);
    else delete (globalThis as { window?: unknown }).window;
});

function task(mode: "image" | "video" = "image"): GenerationTask {
    const output = (id: string) => ({ storageKey: `resource:${id}`, dataUrl: "https://expired.example.test/signed?token=obsolete", width: 640, height: 360 });
    return {
        id: `platform-${mode}`,
        projectId: "platform-canvas",
        provider: "platform",
        type: `canvas_${mode}`,
        status: "succeeded",
        prompt: "镜头",
        attempts: 1,
        createdAt: "",
        updatedAt: "",
        resultJson: JSON.stringify(mode === "image" ? { mode, images: [output("image-0"), output("image-1")] } : { mode, video: output("video-1") }),
    };
}

function node(input: GenerationTask): CanvasNodeData {
    return {
        id: "node",
        title: "镜头",
        type: input.type === "canvas_image" ? CanvasNodeType.Image : CanvasNodeType.Video,
        position: { x: 0, y: 0 },
        width: 320,
        height: 180,
        metadata: { taskId: input.id, status: "loading", assetId: "obsolete-local-asset" },
    };
}

for (const mode of ["image", "video"] as const) {
    test(`Platform ${mode} 结果无需旧 Asset，恢复后保留固定引用与幂等回填`, async () => {
        const input = task(mode);
        const assets = useAssetStore.getState().assets;
        let current = [node(input)];
        let applied = 0;
        const consume = async (value: Parameters<Parameters<typeof consumeGenerationTaskNode>[3]>[0]) => {
            const result = await applyMaterializedGenerationTaskResultToNodes(current, value.task, value.output, value.effectKey, "node");
            current = result.nodes;
            applied += 1;
        };
        const ready = await consumeGenerationTaskNode(input, "node", 0, consume);
        expect(ready.resultState).toBe("READY");
        expect(ready.outputs?.map((item) => item.materializedAssetId)).toEqual(mode === "image" ? [undefined, undefined] : [undefined]);
        expect(current.length).toBe(mode === "image" ? 3 : 1);
        for (const item of current) {
            expect(item.metadata?.assetId).toBeUndefined();
            expect(item.metadata?.content).toStartWith("/api/resources/");
            expect(item.metadata?.storageKey).toStartWith("resource:");
        }
        expect(generationTaskOutputsApplied(current[0], input, current)).toBe(true);
        const reopened = JSON.parse(JSON.stringify(current)) as CanvasNodeData[];
        expect(generationTaskOutputsApplied(reopened[0], input, reopened)).toBe(true);
        await consumeGenerationTaskNode(input, "node", 0, consume);
        expect(applied).toBe(1);
        expect(requests.length).toBe(mode === "image" ? 4 : 2);
        let messages = 0;
        await consumeGenerationTaskMessage(input, "message", ({ resultUrls, resultStorageKeys }) => {
            messages += 1;
            expect(resultUrls.every((url) => url.startsWith("/api/resources/") && !url.includes("token="))).toBe(true);
            expect(resultStorageKeys).toEqual(mode === "image" ? ["resource:image-0", "resource:image-1"] : ["resource:video-1"]);
        });
        await consumeGenerationTaskMessage(input, "message", () => {
            throw new Error("消息不能重复消费");
        });
        expect(messages).toBe(mode === "image" ? 2 : 1);
        expect(useAssetStore.getState().assets).toBe(assets);
        expect([...values.keys()].some((key) => key.includes("asset-store"))).toBe(false);
        expect(generationTaskOutputsApplied({ ...current[0], metadata: { ...current[0].metadata, storageKey: "resource:foreign" } }, input, current)).toBe(false);
    });
}

test("已消费结果恢复时重新验证删除/权限/类型，不能信任旧本地完成状态", async () => {
    const input = task("video");
    const ready = await materializeGenerationTaskAssets(input);
    status = "deleted";
    await expect(materializeGenerationTaskAssets(ready)).rejects.toThrow("平台生成素材尚未就绪");
    status = "ready";
    provider = "local";
    await expect(materializeGenerationTaskAssets(ready)).rejects.toThrow("平台生成素材尚未就绪");
    provider = "platform";
    responseCode = 404;
    await expect(materializeGenerationTaskAssets(ready)).rejects.toThrow("资源不存在或已失效");
});

test("缺少固定资源或类型不符不降级到下载签名 URL/旧 Asset", async () => {
    const input = task("image");
    await expect(materializeGenerationTaskAssets({ ...input, resultJson: JSON.stringify({ images: [{ dataUrl: "https://supplier.example.test/image.png" }] }) })).rejects.toThrow("缺少固定素材引用");
    await expect(materializeGenerationTaskAssets({ ...input, resultJson: JSON.stringify({ images: [{ storageKey: "resource:video-1" }] }) })).rejects.toThrow("平台生成素材尚未就绪");
});

test("校验期间切换账号或取消，不能完成节点消费", async () => {
    const input = task("video");
    duringRead = () => {
        scope = "platform-generation-b";
    };
    await expect(
        consumeGenerationTaskNode(input, "node", 0, () => {
            throw new Error("不可回填新账号");
        }),
    ).rejects.toMatchObject({ name: "AbortError" });
    duringRead = undefined;
    const controller = new AbortController();
    duringRead = () => controller.abort();
    await expect(
        consumeGenerationTaskNode(
            input,
            "node",
            0,
            () => {
                throw new Error("不可回填取消结果");
            },
            { signal: controller.signal },
        ),
    ).rejects.toMatchObject({ name: "AbortError" });
});

test("原生 Pi 已写回的 Platform 节点可直接准备持久化，不要求本地资产", async () => {
    const input = task("video");
    const result = { ...node(input), metadata: { taskId: input.id, storageKey: "resource:video-1", content: "/api/resources/video-1/file", status: "success" as const } };
    withCanvasStorePersistenceSuppressed(() => useCanvasStore.setState({ projects: [{ id: "platform-canvas", name: "画布", nodes: [result], connections: [], createdAt: "", updatedAt: "" }] }));
    expect((await prepareCanvasMediaNodeAsset("platform-canvas", result)).metadata?.storageKey).toBe("resource:video-1");
    expect(requests).toEqual(["/resources/video-1"]);
});

test("真实画布消费先持久化固定引用，清空内存后仍能恢复全部图片", async () => {
    scope = "platform-generation-durable";
    const projectId = useCanvasStore.getState().createProject("平台输出");
    const input = { ...task(), projectId };
    const nodesRef = { current: [node(input)] };
    useCanvasStore.getState().updateProject(projectId, { nodes: nodesRef.current });
    await flushCanvasStorePersistence();
    await consumeGenerationTaskNode(input, "node", 0, async ({ task, output, effectKey, signal }) => {
        await applyCanvasGenerationTaskNodeEffect({
            projectId,
            nodeId: "node",
            task,
            output,
            effectKey,
            signal,
            nodesRef,
            setNodes: (next) => {
                nodesRef.current = typeof next === "function" ? next(nodesRef.current) : next;
            },
        });
    });
    await flushCanvasStorePersistence();
    const read = () => localForageStorageForScope(scope).getItem(CANVAS_STORE_KEY);
    const persisted = parseCanvasStorageDocument(await read()).state.projects.find((item) => item.id === projectId)!;
    expect(persisted.nodes.length).toBe(3);
    expect(generationTaskOutputsApplied(persisted.nodes[0], input, persisted.nodes)).toBe(true);
    expect(persisted.nodes.every((item) => !item.metadata?.assetId && item.metadata?.generationEffectKeys?.length === 1)).toBe(true);
    withCanvasStorePersistenceSuppressed(() => useCanvasStore.setState({ projects: [] }));
    const restored = parseCanvasStorageDocument(await read()).state.projects.find((item) => item.id === projectId)!;
    expect(restored.nodes).toEqual(persisted.nodes);
    expect(JSON.stringify(restored)).not.toContain("expired.example.test");
});
