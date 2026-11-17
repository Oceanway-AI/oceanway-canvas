import type { GenerationTask } from "./api/task-center";
import { resolveResourceAccessURL } from "./api/resources";

export type TaskEarlyPreview = { taskId: string; index: number; kind: "image" | "video" };

export function earlyPreviewURL(ref: TaskEarlyPreview) {
    if (typeof ref.taskId !== "string" || !/^[a-zA-Z0-9_-]+$/.test(ref.taskId) || !Number.isSafeInteger(ref.index) || ref.index < 0 || (ref.kind !== "image" && ref.kind !== "video")) return "";
    return resolveResourceAccessURL(`/api/tasks/${ref.taskId}/previews/${ref.index}`);
}

export function taskEarlyPreviews(task: GenerationTask): TaskEarlyPreview[] {
    if (task.provider !== "platform" || (task.status !== "running" && task.status !== "succeeded")) return [];
    const seen = new Set<number>();
    const refs: TaskEarlyPreview[] = [];
    const slots = Array.isArray(task.earlyPreviews) ? [...task.earlyPreviews] : [];
    for (const slot of slots) {
        if (!slot || seen.has(slot.index)) continue;
        const ref = { taskId: task.id, index: slot.index, kind: slot.kind };
        if (!earlyPreviewURL(ref)) continue;
        seen.add(ref.index);
        refs.push(ref);
    }
    return refs;
}
