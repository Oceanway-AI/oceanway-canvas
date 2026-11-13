import type { InsertAssetPayload } from "@/components/canvas/asset-picker-modal";
import { adoptPlatformMaterial } from "./api/platform-materials";
import { resourceStorageKey, resolveResourceUrl } from "./api/resources";
import { getActiveUserScope } from "@/lib/user-scope";
import { getRemoteAsset, upsertRemoteAsset } from "./api/user-data";
import { ApiError } from "./api/request";
import { resourceIdFromStorageKey } from "./api/resources";
import type { Asset } from "@/stores/use-asset-store";
import { getResourceAccess } from "./api/resources";
import { loadRasterImage } from "@/lib/image-raster";

// A native creative record refers to an already adopted file, without creating
// another upload. Retry keeps the user's existing title/folder/tags intact.
export async function saveAdoptedLibraryAsset(payload: InsertAssetPayload, folderId?: string): Promise<string> {
    if (payload.kind !== "image" && payload.kind !== "video" && payload.kind !== "audio") throw new Error("请选择图片、视频或音频素材");
    const resourceId = resourceIdFromStorageKey(payload.storageKey);
    if (!resourceId) throw new Error("素材尚未完成采用");
    const scope = getActiveUserScope();
    const assertCurrent = () => {
        if (getActiveUserScope() !== scope) throw new DOMException("素材操作已因账号切换取消", "AbortError");
    };
    const id = `platform-${resourceId}`;
    let existingAsset: Asset | undefined;
    try {
        const existing = await getRemoteAsset(id);
        assertCurrent();
        if (existing.asset?.id !== id) throw new Error("创作资产响应不匹配");
        existingAsset = existing.asset;
        if (existingAsset.kind !== payload.kind || !("storageKey" in existingAsset.data) || existingAsset.data.storageKey !== payload.storageKey) throw new Error("创作资产引用不匹配");
        if (existingAsset.kind !== "image" || (existingAsset.data.width > 1 && existingAsset.data.height > 1)) return id;
    } catch (error) {
        assertCurrent();
        if (!(error instanceof ApiError) || (error.status !== 404 && error.code !== 404)) throw error;
    }
    const url = resolveResourceUrl(payload.storageKey);
    let imageWidth = payload.kind === "image" ? payload.width || 0 : 0;
    let imageHeight = payload.kind === "image" ? payload.height || 0 : 0;
    if (payload.kind === "image" && (!imageWidth || !imageHeight)) {
        // SDK file metadata currently omits dimensions. Read the authorized
        // original once; preview dimensions and guessed defaults are incorrect.
        const access = await getResourceAccess(payload.storageKey, "browser-process", "original");
        assertCurrent();
        const image = await loadRasterImage(access.url);
        assertCurrent();
        imageWidth = image.naturalWidth;
        imageHeight = image.naturalHeight;
    }
    const now = new Date().toISOString();
    const base = { id, title: payload.title || "未命名素材", folderId, category: "material" as const, status: "confirmed" as const, tags: [], coverUrl: url, createdAt: now, updatedAt: now, metadata: { source: "platform-library" } };
    const common = { storageKey: payload.storageKey, bytes: payload.bytes || 0, mimeType: payload.mimeType || "" };
    const asset: Asset =
        payload.kind === "image"
            ? { ...base, kind: "image", data: { ...common, dataUrl: url, width: imageWidth, height: imageHeight } }
            : payload.kind === "video"
              ? { ...base, kind: "video", data: { ...common, url, width: payload.width || 0, height: payload.height || 0, durationMs: payload.durationMs || 0 } }
              : { ...base, kind: "audio", data: { ...common, url, durationMs: payload.durationMs || 0 } };
    assertCurrent();
    await upsertRemoteAsset(existingAsset?.kind === "image" ? { ...existingAsset, data: { ...existingAsset.data, width: imageWidth, height: imageHeight } } : asset);
    assertCurrent();
    return id;
}

export async function preparePlatformMaterialInsert(asset: { asset_id: string; title: string; kind: string; version: { version_id: string } }): Promise<InsertAssetPayload> {
    if (!["image", "video", "audio"].includes(asset.kind)) throw new Error("该素材类型暂不能插入画布");
    const scope = getActiveUserScope();
    const resource = await adoptPlatformMaterial(asset.asset_id, asset.version.version_id);
    if (getActiveUserScope() !== scope) throw new DOMException("素材操作已因账号切换取消", "AbortError");
    if (resource.status !== "ready" || resource.provider !== "platform" || resource.kind !== asset.kind || !resource.id) throw new Error("素材尚未就绪");
    const storageKey = resourceStorageKey(resource.id);
    const url = resolveResourceUrl(storageKey);
    const common = { storageKey, title: asset.title, bytes: resource.size, mimeType: resource.mimeType };
    if (resource.kind === "image") return { ...common, kind: "image", dataUrl: url, width: resource.width, height: resource.height };
    if (resource.kind === "video") return { ...common, kind: "video", url, width: resource.width, height: resource.height, durationMs: resource.durationMs };
    return { ...common, kind: "audio", url, durationMs: resource.durationMs };
}

export function insertLibraryAsset(asset: { id: string; title: string; kind: string; primaryVersionId?: string }) {
    if (!asset.primaryVersionId) throw new Error("未找到所选素材版本");
    return preparePlatformMaterialInsert({ asset_id: asset.id, title: asset.title, kind: asset.kind, version: { version_id: asset.primaryVersionId } });
}
