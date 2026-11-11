import type { GenerationTask } from "@/services/api/task-center";
import type { ResourceAccess, ResourceAccessVariant } from "@/services/api/resources";

export type TaskResultMedia = { key: string; kind: "image" | "video"; resourceId?: string; url?: string };

export function taskResultMedia(task: GenerationTask): TaskResultMedia[] {
    let result: unknown;
    try {
        result = JSON.parse(task.resultJson || "null");
    } catch {
        result = task.resultJson;
    }
    if (task.provider === "platform") {
        const items: TaskResultMedia[] = [];
        const add = (resourceId: unknown, kind: unknown) => {
            if (typeof resourceId !== "string" || !/^[a-zA-Z0-9_-]+$/.test(resourceId) || (kind !== "image" && kind !== "video") || items.some((item) => item.resourceId === resourceId)) return;
            items.push({ key: resourceId, resourceId, kind });
        };
        if (task.resultState === "READY") for (const output of task.outputs || []) add(output.materializedResourceId, output.mediaType);
        if (result && typeof result === "object" && !Array.isArray(result)) {
            const value = result as { images?: unknown[]; video?: unknown };
            const read = (entry: unknown, kind: "image" | "video") => {
                if (!entry || typeof entry !== "object") return;
                const storageKey = (entry as { storageKey?: unknown }).storageKey;
                if (typeof storageKey === "string" && storageKey.startsWith("resource:")) add(storageKey.slice(9), kind);
            };
            if (Array.isArray(value.images)) value.images.forEach((entry) => read(entry, "image"));
            read(value.video, "video");
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
