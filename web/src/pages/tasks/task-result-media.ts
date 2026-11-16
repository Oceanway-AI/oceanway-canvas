import type { GenerationTask } from "@/services/api/task-center";
import type { ResourceAccess, ResourceAccessVariant } from "@/services/api/resources";
import { earlyPreviewURL, taskEarlyPreviews, type TaskEarlyPreview } from "@/services/task-early-previews";

export type TaskResultMedia = { key: string; kind: "image" | "video"; resourceId?: string; url?: string; outputIndex?: number; early?: TaskEarlyPreview };

export function withEarlyPreview(task: GenerationTask, item: TaskResultMedia): TaskResultMedia {
    const refs = taskEarlyPreviews(task).filter((ref) => ref.kind === item.kind);
    const early = refs.find((ref) => earlyPreviewURL(ref) === item.url || ref.index === item.outputIndex) || (item.kind === "video" && refs.length === 1 ? refs[0] : undefined);
    return early ? { ...item, early } : item;
}

export function taskResultMedia(task: GenerationTask): TaskResultMedia[] {
    let result: unknown;
    try {
        result = JSON.parse(task.resultJson || "null");
    } catch {
        result = task.resultJson;
    }
    if (task.provider === "platform") {
        const items: TaskResultMedia[] = [];
        const add = (resourceId: unknown, kind: unknown, index?: unknown) => {
            if (typeof resourceId !== "string" || !/^[a-zA-Z0-9_-]+$/.test(resourceId) || (kind !== "image" && kind !== "video") || items.some((item) => item.resourceId === resourceId)) return;
            items.push({ key: resourceId, resourceId, kind, ...(typeof index === "number" && Number.isSafeInteger(index) && index >= 0 ? { outputIndex: index } : {}) });
        };
        if (task.resultState === "READY") for (const output of task.outputs || []) add(output.materializedResourceId, output.mediaType, output.outputIndex);
        if (result && typeof result === "object" && !Array.isArray(result)) {
            const value = result as { images?: unknown[]; video?: unknown };
            const read = (entry: unknown, kind: "image" | "video") => {
                if (!entry || typeof entry !== "object") return;
                const storageKey = (entry as { storageKey?: unknown }).storageKey;
                if (typeof storageKey === "string" && storageKey.startsWith("resource:")) add(storageKey.slice(9), kind, (entry as { outputIndex?: unknown }).outputIndex);
            };
            if (Array.isArray(value.images)) value.images.forEach((entry) => read(entry, "image"));
            read(value.video, "video");
        }
        if (!items.length)
            for (const ref of taskEarlyPreviews(task)) {
                const url = earlyPreviewURL(ref);
                items.push({ key: url, kind: ref.kind, url });
            }
        return items;
    }
    const urls: string[] = [];
    const visit = (item: unknown, key = "") => {
        if (typeof item === "string") {
            const inline = /^(data:image\/|data:video\/)/.test(item);
            const path = /\.(png|jpe?g|webp|gif|avif|mp4|webm|mov)(?:$|\?)/i.test(item);
            const named = /^(https?:|blob:)/.test(item) && /(url|image|video|result|output|media)/i.test(key);
            if ((inline || path || named) && !urls.includes(item)) urls.push(item);
        } else if (Array.isArray(item)) item.forEach((value) => visit(value, key));
        else if (item && typeof item === "object") Object.entries(item).forEach(([field, value]) => visit(value, field));
    };
    visit(result);
    return urls.slice(0, 12).map((url) => ({ key: url, url, kind: url.startsWith("data:video/") || /\.(mp4|webm|mov)(?:$|\?)/i.test(url) || task.type.includes("video") ? "video" : "image" }));
}

export async function taskResultMediaAccess(item: TaskResultMedia, variant: ResourceAccessVariant, read: (storageKey: string, purpose: "display", variant: ResourceAccessVariant) => Promise<ResourceAccess>) {
    if (!item.resourceId) return item.url || "";
    const access = await read(`resource:${item.resourceId}`, "display", variant);
    if (access.resourceId !== item.resourceId || !access.url) throw new Error("素材访问身份不匹配");
    return access.url;
}

export function taskPlaybackPreview(task: GenerationTask): TaskResultMedia | undefined {
    if (task.provider !== "platform") return;
    // 列表摘要可能仅包含 Studio 的稳定资源入口，详情才返回 resultJson。
    const id = /^\/api\/resources\/([a-zA-Z0-9_-]+)\/file(?:\?|$)/.exec(task.previewUrl || "")?.[1];
    if (id && task.previewKind === "video") return withEarlyPreview(task, { key: id, resourceId: id, kind: "video" });
    const video = taskResultMedia(task).find((item) => item.kind === "video");
    if (video) return withEarlyPreview(task, video);
}
