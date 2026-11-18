import { STUDIO_STORAGE_NAME } from "../src/lib/studio-identity";
import { expect, test } from "bun:test";
import { apiClient } from "../src/services/api/request";
import { uploadResourceFile, clearResourceAccessCache } from "../src/services/api/resources";
import { ACTIVE_USER_SCOPE_KEY, getActiveUserScope, setActiveUserScope } from "../src/lib/user-scope";

test("a reopened upload keeps confirmed parts and renews the original session for the same account", async () => {
    const adapter = apiClient.defaults.adapter;
    const previousFetch = globalThis.fetch;
    const previousWindow = Object.getOwnPropertyDescriptor(globalThis, "window");
    const previousScope = getActiveUserScope();
    const values = new Map<string, string>();
    const storage = { getItem: (key: string) => values.get(key) ?? null, setItem: (key: string, value: string) => values.set(key, value), removeItem: (key: string) => values.delete(key) };
    const calls: string[] = [];
    const puts: string[] = [];
    const completedOwners = new Set<string>();
    let interrupted = false;
    const session = {
        id: "resume_local",
        upload: {
            protocol: "oceanway.materials.v2",
            upload_id: "resume_original",
            state: "awaiting_upload",
            byte_length: "6",
            part_size: "3",
            tickets: [1, 2].map((part_number) => ({ part_number, method: "PUT", url: `https://objects.example.test/${part_number}?signed=ephemeral`, headers: {}, expires_at: "2099-01-01T00:00:00Z" })),
        },
    };
    try {
        Object.defineProperty(globalThis, "window", { configurable: true, value: { localStorage: storage } });
        setActiveUserScope("upload-owner-a");
        apiClient.defaults.adapter = async (config) => {
            calls.push(`${config.method} ${config.url}`);
            let data: unknown = session;
            if (config.url?.endsWith("/complete")) {
                expect(JSON.parse(config.data).parts).toEqual([
                    { part_number: 1, etag: "etag-1" },
                    { part_number: 2, etag: "etag-2" },
                ]);
                completedOwners.add(getActiveUserScope());
            }
            if (completedOwners.has(getActiveUserScope())) data = { ...session, upload: { ...session.upload, state: "ready" }, resource: { id: "resume_resource", provider: "platform", status: "ready" } };
            return { config, status: 200, statusText: "OK", headers: {}, data: { code: 0, data } };
        };
        globalThis.fetch = (async (url) => {
            const part = new URL(String(url)).pathname.slice(1);
            puts.push(part);
            if (part === "2" && !interrupted) {
                interrupted = true;
                throw new TypeError("tab closed during second part");
            }
            return new Response(null, { status: 200, headers: { ETag: `etag-${part}` } });
        }) as typeof fetch;
        const meta = { fileName: "resume.txt", idempotencyKey: "canvas:project-a:node-a" };
        await expect(uploadResourceFile(new Blob(["abcdef"]), "file", meta)).rejects.toThrow();
        const checkpoints = [...values.entries()].filter(([key]) => key !== ACTIVE_USER_SCOPE_KEY);
        expect(checkpoints).toHaveLength(1);
        expect(checkpoints[0][1]).toContain("etag-1");
        expect(checkpoints[0][1]).not.toContain("ephemeral");
        expect(checkpoints[0][1]).not.toContain("https:");
        // The new client instance has no previous HTTP or resource-cache state.
        clearResourceAccessCache();
        const result = await uploadResourceFile(new Blob(["abcdef"]), "file", meta);
        expect(result.id).toBe("resume_resource");
        expect(puts).toEqual(["1", "2", "2"]);
        expect(calls.filter((call) => call === "post /resources/platform-uploads")).toHaveLength(1);
        expect(calls).toContain("get /resources/platform-uploads/resume_local");
        clearResourceAccessCache();
        expect((await uploadResourceFile(new Blob(["abcdef"]), "file", meta)).id).toBe("resume_resource");
        expect(puts).toEqual(["1", "2", "2"]);
        expect(calls.filter((call) => call === "post /resources/platform-uploads")).toHaveLength(1);
        const readyCheckpoint = [...values.entries()].find(([key]) => key.startsWith(`${STUDIO_STORAGE_NAME}:studio-material-upload:`));
        expect(JSON.parse(readyCheckpoint![1]).parts).toEqual([]);
        const beforeConflict = calls.length;
        await expect(uploadResourceFile(new Blob(["abcdefg"]), "file", meta)).rejects.toThrow("原上传");
        expect(calls).toHaveLength(beforeConflict);
        setActiveUserScope("upload-owner-b");
        await uploadResourceFile(new Blob(["abcdef"]), "file", meta);
        expect(calls.filter((call) => call === "post /resources/platform-uploads")).toHaveLength(2);
        expect(puts).toEqual(["1", "2", "2", "1", "2"]);
    } finally {
        setActiveUserScope(previousScope);
        if (previousWindow) Object.defineProperty(globalThis, "window", previousWindow);
        else Reflect.deleteProperty(globalThis, "window");
        apiClient.defaults.adapter = adapter;
        globalThis.fetch = previousFetch;
    }
});

test("Platform upload uses credential-free tickets and recovers a lost completion without a new command", async () => {
    const adapter = apiClient.defaults.adapter;
    const fetchBefore = globalThis.fetch;
    const calls: string[] = [];
    let completed = false;
    let command = "";
    const progress: number[][] = [];
    try {
        apiClient.defaults.adapter = async (config) => {
            calls.push(`${config.method} ${config.url}`);
            let data: unknown;
            if (config.url === "/resources/platform-uploads") {
                const input = JSON.parse(config.data);
                command = input.commandId;
                expect(input.sha256).toBe("2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824");
                data = {
                    id: "local_1",
                    upload: {
                        protocol: "oceanway.materials.v2",
                        upload_id: "up_1",
                        state: "awaiting_upload",
                        byte_length: "5",
                        part_size: "5",
                        tickets: [{ part_number: 1, method: "PUT", url: "https://objects.example.test/ticket", headers: { "Content-Type": "text/plain" }, expires_at: "2099-01-01T00:00:00Z" }],
                    },
                };
            } else if (config.url?.endsWith("/complete")) {
                completed = true;
                expect(JSON.parse(config.data)).toEqual({ parts: [{ part_number: 1, etag: '"etag-1"' }] });
                throw new Error("connection lost after commit");
            } else if (config.url === "/resources/platform-uploads/local_1") {
                expect(completed).toBe(true);
                data = {
                    id: "local_1",
                    upload: {
                        protocol: "oceanway.materials.v2",
                        upload_id: "up_1",
                        state: "ready",
                        asset: { asset_id: "asset1", version: { asset_id: "asset1", version_id: "version1" }, file: { sha256: "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824", byte_length: "5" } },
                    },
                };
            } else if (config.url === "/resources/adopt") {
                expect(JSON.parse(config.data)).toMatchObject({ assetId: "asset1", versionId: "version1" });
                data = { resource: { id: "resource_1", status: "ready", provider: "platform" } };
            } else throw new Error(`Unexpected API ${config.url}`);
            return { config, status: 200, statusText: "OK", headers: {}, data: { code: 0, data } };
        };
        globalThis.fetch = (async (url, init) => {
            expect(String(url)).toBe("https://objects.example.test/ticket");
            expect(init?.credentials).toBe("omit");
            expect(init?.redirect).toBe("error");
            expect(new Headers(init?.headers).get("Cookie")).toBeNull();
            expect(await (init?.body as Blob).text()).toBe("hello");
            return new Response(null, { status: 200, headers: { ETag: '"etag-1"' } });
        }) as typeof fetch;
        const resource = await uploadResourceFile(new Blob(["hello"], { type: "text/plain" }), "file", { fileName: "hello.txt", idempotencyKey: "logical:upload:1" }, (loaded, total) => progress.push([loaded, total]));
        expect(resource.id).toBe("resource_1");
        expect(command).toMatch(/^[a-zA-Z0-9_-]+$/);
        expect(calls).toEqual(["post /resources/platform-uploads", "post /resources/platform-uploads/local_1/complete", "get /resources/platform-uploads/local_1", "post /resources/adopt"]);
        expect(progress).toEqual([[5, 5]]);
    } finally {
        apiClient.defaults.adapter = adapter;
        globalThis.fetch = fetchBefore;
    }
});

test("a lost create receipt preserves the original filename and metadata when local media sync resumes", async () => {
    const adapter = apiClient.defaults.adapter;
    const previousWindow = Object.getOwnPropertyDescriptor(globalThis, "window");
    const values = new Map<string, string>();
    const bodies: unknown[] = [];
    try {
        Object.defineProperty(globalThis, "window", { configurable: true, value: { localStorage: { getItem: (key: string) => values.get(key) ?? null, setItem: (key: string, value: string) => values.set(key, value) } } });
        apiClient.defaults.adapter = async (config) => {
            expect(config.url).toBe("/resources/platform-uploads");
            bodies.push(JSON.parse(config.data));
            if (bodies.length === 1) throw new Error("create committed but receipt was lost");
            return {
                config,
                status: 200,
                statusText: "OK",
                headers: {},
                data: { code: 0, data: { id: "host-original", upload: { protocol: "oceanway.materials.v2", upload_id: "original", state: "ready" }, resource: { id: "resource", provider: "platform", status: "ready" } } },
            };
        };
        const file = new Blob(["same bytes"], { type: "image/png" });
        await expect(uploadResourceFile(file, "image", { idempotencyKey: "lost-create", fileName: "my-file.png", width: 5, height: 2 })).rejects.toThrow();
        await uploadResourceFile(file, "image", { idempotencyKey: "lost-create" });
        expect(bodies).toHaveLength(2);
        expect(bodies[1]).toEqual(bodies[0]);
        expect(bodies[1]).toMatchObject({ filename: "my-file.png", width: 5, height: 2 });
    } finally {
        if (previousWindow) Object.defineProperty(globalThis, "window", previousWindow);
        else Reflect.deleteProperty(globalThis, "window");
        apiClient.defaults.adapter = adapter;
    }
});

test("corrupt recovery records and unavailable checkpoint storage fail before creating an upload", async () => {
    const adapter = apiClient.defaults.adapter;
    const previousWindow = Object.getOwnPropertyDescriptor(globalThis, "window");
    let calls = 0;
    try {
        apiClient.defaults.adapter = async () => {
            calls++;
            throw new Error("must not send HTTP");
        };
        for (const raw of ["not json", "null", "{}", '{"fingerprint":"value","filename":"file","parts":[null],"meta":{}}']) {
            Object.defineProperty(globalThis, "window", { configurable: true, value: { localStorage: { getItem: () => raw, setItem: () => {} } } });
            await expect(uploadResourceFile(new Blob(["x"]), "file", { idempotencyKey: "corrupt-record" })).rejects.toMatchObject({ permanent: true });
        }
        Object.defineProperty(globalThis, "window", {
            configurable: true,
            value: {
                localStorage: {
                    getItem: () => null,
                    setItem: () => {
                        throw new Error("storage full");
                    },
                },
            },
        });
        await expect(uploadResourceFile(new Blob(["x"]), "file", { idempotencyKey: "storage-failure" })).rejects.toMatchObject({ permanent: true });
        expect(calls).toBe(0);
    } finally {
        if (previousWindow) Object.defineProperty(globalThis, "window", previousWindow);
        else Reflect.deleteProperty(globalThis, "window");
        apiClient.defaults.adapter = adapter;
    }
});

test("Switching accounts after upload preparation cannot send ticket bytes or complete the upload", async () => {
    const adapter = apiClient.defaults.adapter;
    const fetchBefore = globalThis.fetch;
    try {
        apiClient.defaults.adapter = async (config) => {
            clearResourceAccessCache();
            return { config, status: 200, statusText: "OK", headers: {}, data: { code: 0, data: { id: "u", upload: { protocol: "oceanway.materials.v2", state: "awaiting_upload" } } } };
        };
        globalThis.fetch = (() => {
            throw new Error("must never upload after account switch");
        }) as typeof fetch;
        await expect(uploadResourceFile(new Blob(["x"]), "file")).rejects.toMatchObject({ name: "AbortError" });
    } finally {
        apiClient.defaults.adapter = adapter;
        globalThis.fetch = fetchBefore;
    }
});

test("Untrusted ticket headers and expired URLs are rejected before disclosing file bytes", async () => {
    const adapter = apiClient.defaults.adapter;
    const fetchBefore = globalThis.fetch;
    let fetches = 0;
    try {
        globalThis.fetch = (() => {
            fetches++;
            throw new Error("must not upload");
        }) as typeof fetch;
        for (const ticket of [
            { headers: { Cookie: "secret" }, expires_at: "2099-01-01T00:00:00Z" },
            { headers: { "X-Platform-Product-Key": "secret" }, expires_at: "2099-01-01T00:00:00Z" },
            { headers: {}, expires_at: "2000-01-01T00:00:00Z" },
            { url: "http://127.0.0.1:18474/_local-material-upload/key", headers: {}, expires_at: "2099-01-01T00:00:00Z" },
            { url: "http://objects.example.test/_local-material-upload/key", headers: {}, expires_at: "2099-01-01T00:00:00Z" },
        ]) {
            apiClient.defaults.adapter = async (config) => ({
                config,
                status: 200,
                statusText: "OK",
                headers: {},
                data: {
                    code: 0,
                    data: {
                        id: "local",
                        upload: { protocol: "oceanway.materials.v2", upload_id: "original", state: "awaiting_upload", byte_length: "1", part_size: "1", tickets: [{ part_number: 1, method: "PUT", url: "https://objects.example.test/upload", ...ticket }] },
                    },
                },
            });
            await expect(uploadResourceFile(new Blob(["x"]), "file")).rejects.toThrow("上传票据");
        }
        expect(fetches).toBe(0);
    } finally {
        apiClient.defaults.adapter = adapter;
        globalThis.fetch = fetchBefore;
    }
});
