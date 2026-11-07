import { expect, test } from "bun:test";
import { apiClient } from "../src/services/api/request";
import { uploadResourceFile, clearResourceAccessCache } from "../src/services/api/resources";

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
                data = { id: "local_1", upload: { protocol: "oceanway.materials.v2", upload_id: "up_1", state: "ready" }, resource: { id: "resource_1", status: "ready", provider: "platform" } };
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
        expect(calls).toEqual(["post /resources/platform-uploads", "post /resources/platform-uploads/local_1/complete", "get /resources/platform-uploads/local_1"]);
        expect(progress).toEqual([[5, 5]]);
    } finally {
        apiClient.defaults.adapter = adapter;
        globalThis.fetch = fetchBefore;
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
