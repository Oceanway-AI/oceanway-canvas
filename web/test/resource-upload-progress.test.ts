import { expect, test } from "bun:test";
import { apiClient } from "../src/services/api/request";
import { uploadResourceFile } from "../src/services/api/resources";

test("票据上传按已确认的分片累计真实字节，100% 后仍须平台确认", async () => {
    const adapter = apiClient.defaults.adapter;
    const previousFetch = globalThis.fetch;
    const progress: number[][] = [];
    let completed = false;
    try {
        apiClient.defaults.adapter = async (config) => {
            let data: unknown;
            if (config.url === "/resources/platform-uploads") {
                data = {
                    id: "local",
                    upload: {
                        protocol: "oceanway.materials.v2",
                        upload_id: "original",
                        state: "awaiting_upload",
                        byte_length: "5",
                        part_size: "3",
                        tickets: [1, 2].map((part_number) => ({ part_number, method: "PUT", url: `https://objects.example.test/${part_number}`, headers: {}, expires_at: "2099-01-01T00:00:00Z" })),
                    },
                };
            } else {
                expect(config.url).toBe("/resources/platform-uploads/local/complete");
                expect(progress.at(-1)).toEqual([5, 5]);
                completed = true;
                data = { id: "local", upload: { protocol: "oceanway.materials.v2", upload_id: "original", state: "ready" }, resource: { id: "resource", provider: "platform", status: "ready" } };
            }
            return { config, status: 200, statusText: "OK", headers: {}, data: { code: 0, data } };
        };
        globalThis.fetch = (async (_url, init) => {
            expect(completed).toBe(false);
            expect((init?.body as Blob).size).toBe(progress.length === 0 ? 3 : 2);
            return new Response(null, { status: 200, headers: { ETag: "part" } });
        }) as typeof fetch;
        await uploadResourceFile(new Blob(["hello"]), "file", undefined, (loaded, total) => progress.push([loaded, total]));
        expect(progress).toEqual([
            [3, 5],
            [5, 5],
        ]);
        expect(completed).toBe(true);
    } finally {
        apiClient.defaults.adapter = adapter;
        globalThis.fetch = previousFetch;
    }
});

test("传输结束不吞掉服务端保存错误", async () => {
    const adapter = apiClient.defaults.adapter;
    try {
        apiClient.defaults.adapter = async (config) => {
            return { config, status: 200, statusText: "OK", headers: {}, data: { code: 9, msg: "保存失败" } };
        };
        await expect(uploadResourceFile(new Blob(["text"]), "file", undefined, () => {})).rejects.toThrow("保存失败");
    } finally {
        apiClient.defaults.adapter = adapter;
    }
});
