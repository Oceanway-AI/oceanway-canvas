import { afterEach, beforeEach, expect, test } from "bun:test";
import { apiClient, ApiError } from "@/services/api/request";
import { clearResourceAccessCache } from "@/services/api/resources";
import { setActiveUserScope, getActiveUserScope } from "@/lib/user-scope";
import { preparePlatformCreationAssets } from "@/services/creation-platform-handoff";
import type { Asset } from "@/stores/use-asset-store";

const prior = apiClient.defaults.adapter;
const previousScope = getActiveUserScope();
const previousWindow = Object.getOwnPropertyDescriptor(globalThis, "window");
let saved: Map<string, Asset>;
let writes: string[];
let status: string;
let loseResponse: boolean;
let switchAccount: boolean;
const messages = [
    { mode: "image", content: "图片", resultStorageKeys: ["resource:image1", "resource:image2"], resultUrls: ["https://expired.test/?sign=old", ""] },
    { mode: "video", content: "视频", resultStorageKeys: ["resource:video1"] },
];
beforeEach(() => {
    const values = new Map<string, string>();
    Object.defineProperty(globalThis, "window", { configurable: true, value: { localStorage: { getItem: (key: string) => values.get(key) ?? null, setItem: (key: string, value: string) => values.set(key, value) }, setTimeout, clearTimeout } });
    saved = new Map();
    writes = [];
    status = "ready";
    loseResponse = false;
    switchAccount = false;
    setActiveUserScope("a2-owner");
    clearResourceAccessCache();
    apiClient.defaults.adapter = async (config) => {
        const path = config.url || "";
        let data: unknown;
        if (config.method === "get" && path.startsWith("/resources/")) {
            const id = path.split("/").at(-1)!;
            if (switchAccount) setActiveUserScope("a2-other");
            data = { resource: { id, provider: "platform", status, kind: id.startsWith("image") ? "image" : "video", width: 640, height: 360, size: 128, mimeType: id.startsWith("image") ? "image/png" : "video/mp4" } };
        } else if (path.startsWith("/assets/")) {
            const id = path.split("/").at(-1)!;
            if (config.method === "put") {
                const asset = JSON.parse(config.data).asset;
                saved.set(id, asset);
                writes.push(id);
                if (loseResponse && id.endsWith("image2")) {
                    loseResponse = false;
                    throw new ApiError("保存响应中断", { status: 503 });
                }
            } else if (config.method !== "get") throw new Error("unexpected asset mutation");
            if (!saved.has(id)) throw new ApiError("不存在", { status: 404 });
            data = { asset: saved.get(id) };
        } else throw new Error(`forbidden model/upload/binding call: ${config.method} ${path}`);
        return { config, status: 200, statusText: "OK", headers: {}, data: { code: 0, data } };
    };
});
afterEach(() => {
    apiClient.defaults.adapter = prior;
    clearResourceAccessCache();
    setActiveUserScope(previousScope);
    if (previousWindow) Object.defineProperty(globalThis, "window", previousWindow);
    else delete (globalThis as { window?: unknown }).window;
});

test("existing image/video results are explicitly adopted once without uploading or generating", async () => {
    const expected = ["platform-image1", "platform-image2", "platform-video1"];
    expect(await preparePlatformCreationAssets(messages)).toEqual(expected);
    saved.get(expected[0])!.title = "保留编辑";
    expect(await preparePlatformCreationAssets(messages)).toEqual(expected);
    expect(writes).toEqual(expected);
    expect(saved.get(expected[0])!.title).toBe("保留编辑");
    expect(JSON.stringify([...saved.values()])).not.toContain("sign=");
});
test("lost save response and partial adoption recover the same IDs and preserve existing edits", async () => {
    loseResponse = true;
    await expect(preparePlatformCreationAssets(messages)).rejects.toThrow("保存响应中断");
    expect(saved.size).toBe(2);
    saved.get("platform-image2")!.title = "中断后编辑";
    expect(await preparePlatformCreationAssets(messages)).toEqual(["platform-image1", "platform-image2", "platform-video1"]);
    expect(writes).toEqual(["platform-image1", "platform-image2", "platform-video1"]);
    expect(saved.get("platform-image2")!.title).toBe("中断后编辑");
});
test("not-ready and mismatched Resources reject before any business writes", async () => {
    status = "pending";
    await expect(preparePlatformCreationAssets(messages)).rejects.toThrow("尚未就绪");
    status = "ready";
    await expect(preparePlatformCreationAssets([{ mode: "video", resultStorageKeys: ["resource:image1"] }])).rejects.toThrow("类型不匹配");
    expect(writes).toEqual([]);
});
test("account switch cannot adopt the previous user's files", async () => {
    switchAccount = true;
    await expect(preparePlatformCreationAssets(messages)).rejects.toThrow("账号切换");
    expect(writes).toEqual([]);
});
test("a single selected message does not adopt other conversation outputs", async () => {
    expect(await preparePlatformCreationAssets([messages[1]])).toEqual(["platform-video1"]);
    expect(writes).toEqual(["platform-video1"]);
});
test("expired URL-only history cannot manufacture a fixed file identity", async () => {
    await expect(preparePlatformCreationAssets([{ mode: "image", resultUrls: ["https://expired.test/?sign=old"] }])).rejects.toThrow("固定素材引用");
    expect(writes).toEqual([]);
});
