import { afterEach, expect, test } from "bun:test";
import { apiClient } from "@/services/api/request";
import { getPlatformMaterialAccess, type PlatformMaterialAsset } from "@/services/api/platform-materials";
import { saveAdoptedLibraryAsset } from "@/services/platform-material-selection";
import { AssetMediaPreview } from "@/components/asset-media-preview";
import type { Asset } from "@/stores/use-asset-store";

const previous = apiClient.defaults.adapter;
afterEach(() => { apiClient.defaults.adapter = previous; });

test("shared file preview is a read and never adopts or uploads", async () => {
    const calls: string[] = [];
    apiClient.defaults.adapter = async (config) => {
        calls.push(`${config.method} ${config.url}`);
        expect(JSON.parse(config.data)).toEqual({ items: [{ item_id: "view", asset_id: "a", version_id: "v", purpose: "display", variant: "preview" }] });
        return { config, status: 200, statusText: "OK", headers: {}, data: { code: 0, data: { protocol: "oceanway.materials.v2", results: [{ item_id: "view", asset_id: "a", version_id: "v", file_id: "f", status: "ready", requested_variant: "preview", actual_variant: "preview", url: "https://cdn.test/preview?sign=private", expires_at: "2099-01-01T00:00:00Z" }] } } };
    };
    const access = await getPlatformMaterialAccess({ asset_id: "a", kind: "image", version: { version_id: "v" }, file: { file_id: "f" } } as PlatformMaterialAsset);
    expect(access.url).toStartWith("https://cdn.test/");
    expect(calls).toEqual(["post /materials/access"]);
});

test("adopted file becomes one native business asset; repeated adoption preserves edits", async () => {
    let saved: Asset | undefined;
    let writes = 0;
    apiClient.defaults.adapter = async (config) => {
        expect(config.url).toBe("/assets/platform-r1");
        if (config.method === "put") { saved = JSON.parse(config.data).asset; writes++; }
        return { config, status: 200, statusText: "OK", headers: {}, data: saved ? { code: 0, data: { asset: saved } } : { code: 404, data: null, msg: "not found" } };
    };
    const payload = { kind: "image" as const, title: "共享图片", storageKey: "resource:r1", dataUrl: "https://cdn.test/temporary?sign=private", width: 32, height: 24, bytes: 8, mimeType: "image/png" };
    expect(await saveAdoptedLibraryAsset(payload, "folder-1")).toBe("platform-r1");
    expect(saved).toMatchObject({ kind: "image", folderId: "folder-1", data: { storageKey: "resource:r1" } });
    expect(JSON.stringify(saved)).not.toContain("sign=");
    saved!.title = "用户修改的标题";
    await saveAdoptedLibraryAsset(payload, "folder-2");
    expect(writes).toBe(1);
    expect(saved!.title).toBe("用户修改的标题");
});

test.each(["image", "video"] as const)("native %s cards request small derived files", (kind) => {
    const asset = { id: "a", kind, title: "media", coverUrl: "", tags: [], data: { storageKey: "resource:r1", dataUrl: "/api/resources/r1/file", url: "/api/resources/r1/file" } } as unknown as Asset;
    const preview = AssetMediaPreview({ asset, alt: "media", thumbnail: true }) as React.ReactElement<{ variant: string; storageKey: string }>;
    expect(preview.props.variant).toBe(kind === "image" ? "thumbnail" : "poster");
    expect(preview.props.storageKey).toBe("resource:r1");
});
