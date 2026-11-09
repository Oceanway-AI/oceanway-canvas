import { getActiveUserScope } from "@/lib/user-scope";
import { refreshResource, resourceFileUrl } from "./api/resources";
import { saveAdoptedLibraryAsset } from "./platform-material-selection";

type ResultMessage = { mode?: string; content?: string; resultStorageKeys?: string[]; resultUrls?: string[] };

// This is explicit business adoption, after the original task has saved its
// files. It never submits a model, uploads bytes or creates a new file binding.
export async function preparePlatformCreationAssets(messages: ResultMessage[]): Promise<string[]> {
    const scope = getActiveUserScope();
    const assertCurrent = () => {
        if (scope !== getActiveUserScope()) throw new DOMException("素材操作已因账号切换取消", "AbortError");
    };
    const results = new Map<string, { kind: "image" | "video"; title: string }>();
    for (const message of messages) {
        const keys = message.resultStorageKeys || [];
        if (!keys.length && !message.resultUrls?.length) continue;
        if (!keys.length || (message.mode !== "image" && message.mode !== "video") || (message.resultUrls?.length || 0) > keys.length) throw new Error("生成结果缺少固定素材引用，请先恢复原任务结果");
        for (const key of keys) {
            const id = /^resource:([^\s/:?#]+)$/.exec(key)?.[1];
            if (!id || (results.has(id) && results.get(id)!.kind !== message.mode)) throw new Error("生成结果素材引用不匹配");
            results.set(id, { kind: message.mode, title: message.content?.trim() || "生成素材" });
        }
    }
    // Validate the whole selection before the first business write.
    const resources = await Promise.all(
        [...results].map(async ([id, expected]) => {
            const resource = await refreshResource(id);
            assertCurrent();
            if (resource.id !== id || resource.provider !== "platform" || resource.status !== "ready" || resource.kind !== expected.kind) throw new Error("生成素材尚未就绪或类型不匹配");
            return { resource, ...expected };
        }),
    );
    const ids: string[] = [];
    for (const { resource, kind, title } of resources) {
        assertCurrent();
        const common = { title, storageKey: `resource:${resource.id}`, width: resource.width, height: resource.height, bytes: resource.size, mimeType: resource.mimeType };
        ids.push(await saveAdoptedLibraryAsset(kind === "image" ? { ...common, kind, dataUrl: resourceFileUrl(resource.id) } : { ...common, kind, url: resourceFileUrl(resource.id), durationMs: resource.durationMs }));
    }
    assertCurrent();
    return ids;
}
