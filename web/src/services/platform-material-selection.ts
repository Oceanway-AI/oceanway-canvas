import type { InsertAssetPayload } from "@/components/canvas/asset-picker-modal";
import { adoptPlatformMaterial } from "./api/platform-materials";
import { resourceStorageKey, resolveResourceUrl } from "./api/resources";
import { getActiveUserScope } from "@/lib/user-scope";

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
