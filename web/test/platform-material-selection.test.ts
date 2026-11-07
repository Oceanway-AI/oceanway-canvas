import { expect, test } from "bun:test";
import { apiClient } from "../src/services/api/request";
import { preparePlatformMaterialInsert } from "../src/services/platform-material-selection";
import type { PlatformMaterialAsset } from "../src/services/api/platform-materials";

test("Canvas insertion adopts the selected immutable version and stores a stable resource URL", async () => {
    const previous = apiClient.defaults.adapter;
    const asset = { asset_id: "asset_1", title: "Frame", kind: "image", current_version_id: "newer", version: { version_id: "selected", asset_id: "asset_1", file_id: "f" } } as PlatformMaterialAsset;
    try {
        apiClient.defaults.adapter = async (config) => {
            expect(config.url).toBe("/resources/adopt");
            expect(JSON.parse(config.data)).toEqual({ assetId: "asset_1", versionId: "selected" });
            return { config, status: 200, statusText: "OK", headers: {}, data: { code: 0, data: { resource: { id: "r1", provider: "platform", kind: "image", status: "ready", mimeType: "image/png", size: 8 } } } };
        };
        const payload = await preparePlatformMaterialInsert(asset);
        expect(payload.kind).toBe("image");
        expect("storageKey" in payload && payload.storageKey).toBe("resource:r1");
        expect("dataUrl" in payload && payload.dataUrl).toContain("/resources/r1/file");
        expect(JSON.stringify(payload)).not.toContain("signature=");
        expect(payload.assetId).toBeUndefined();
    } finally {
        apiClient.defaults.adapter = previous;
    }
});
