import type { InsertAssetPayload } from "@/components/canvas/asset-picker-modal";
import { getActiveUserScope } from "@/lib/user-scope";
import { refreshResource, resourceFileUrl, resourceStorageKey } from "@/services/api/resources";

// Revalidate ownership and readiness on every handoff, including after refresh.
// Persist fixed Resource references, never signed URLs or duplicate Asset rows.
export async function loadCanvasResourceHandoffPayloads(ids: string[]): Promise<InsertAssetPayload[]> {
    const scope = getActiveUserScope();
    return Promise.all(
        [...new Set(ids)].map(async (id): Promise<InsertAssetPayload> => {
            if (!id || /[\s/:?#]/.test(id)) throw new Error("无效的素材引用");
            const resource = await refreshResource(id);
            if (scope !== getActiveUserScope()) throw new DOMException("账号已切换", "AbortError");
            if (resource.id !== id || resource.provider !== "platform" || resource.status !== "ready" || (resource.kind !== "image" && resource.kind !== "video")) throw new Error("平台素材尚未就绪或类型不匹配");
            const common = { storageKey: resourceStorageKey(id), title: resource.kind === "image" ? "生成图片" : "生成视频", width: resource.width, height: resource.height, bytes: resource.size, mimeType: resource.mimeType };
            return resource.kind === "image" ? { ...common, kind: "image", dataUrl: resourceFileUrl(id) } : { ...common, kind: "video", url: resourceFileUrl(id), durationMs: resource.durationMs };
        }),
    );
}
