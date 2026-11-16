import type { GenerationTask } from "@/services/api/task-center";
import type { CanvasNodeData, CanvasNodeMetadata } from "@/types/canvas";
import { resolveResourceUrl } from "@/services/api/resources";

export type StudioGenerationSlot = {
    index: number;
    kind: "image" | "video";
    contentState: string;
    previewState: string;
    materialState: string;
    previewUrl?: string;
    resourceId?: string;
    media?: { storageKey: string; dataUrl: string; mimeType?: string; bytes?: number; width?: number; height?: number; durationMs?: number };
};

export function studioCanvasOutputs(task: GenerationTask): StudioGenerationSlot[] {
    if (!task.platformGenerationRequest || !task.resultJson) return [];
    const result = JSON.parse(task.resultJson) as { outputs?: StudioGenerationSlot[] };
    return Array.isArray(result.outputs) ? result.outputs : [];
}

export function projectStudioCanvasOutput(nodes: CanvasNodeData[], rootId: string, task: GenerationTask, slot: StudioGenerationSlot) {
    const root = nodes.find((n) => n.id === rootId);
    const unchanged = { nodes, target: undefined as CanvasNodeData | undefined, effectKey: "" };
    if (!root || root.metadata?.taskId !== task.id || !Number.isSafeInteger(slot.index) || slot.index < 0) return unchanged;
    const bound = root.metadata.platformOutputRequest === task.platformGenerationRequest ? root.metadata.platformOutputNodeIds || {} : {};
    const id = slot.index === 0 ? rootId : `${rootId}:task:${task.id}:image:${slot.index}`;
    const previous = nodes.find((n) => n.id === id);
    if ((bound[slot.index] && !previous) || (previous?.metadata?.taskId && previous.metadata.taskId !== task.id)) return unchanged;
    if (previous?.metadata?.platformOutputRequest === task.platformGenerationRequest && previous?.metadata?.platformOutputRevision && previous?.metadata?.platformOutputRevision > (task.platformRevision || 0)) return unchanged;
    const fixed = slot.resourceId && slot.media?.storageKey === "resource:" + slot.resourceId ? slot.media : undefined;
    if (!fixed && previous?.metadata?.platformOutputRequest === task.platformGenerationRequest && previous?.metadata?.storageKey) return unchanged;
    const preview = Boolean(slot.previewUrl && slot.previewState === "available");
    const state = fixed ? "saved" : [slot.materialState, preview ? "playable" : "waiting"].join("-");
    const effectKey = `studio-output:${task.id}:${id}:${slot.index}:${fixed?.storageKey || state}`;
    if (previous?.metadata?.generationEffectKeys?.includes(effectKey)) return unchanged;
    const node: CanvasNodeData = previous || {
        id,
        type: root.type,
        title: `${root.title} · ${slot.index + 1}`,
        position: { x: root.position.x + (root.width + 32) * slot.index, y: root.position.y },
        width: root.width,
        height: root.height,
        metadata: { taskId: task.id, batchRootId: rootId },
    };
    const metadata: CanvasNodeMetadata = {
        ...node.metadata,
        taskId: task.id,
        taskProvider: "platform",
        taskStatus: task.status,
        platformOutputIndex: slot.index,
        platformOutputRequest: task.platformGenerationRequest,
        platformOutputRevision: task.platformRevision,
        generationEffectKeys: [...(node.metadata?.generationEffectKeys || []), effectKey],
        taskEarlyPreviews: preview ? [{ taskId: task.id, index: slot.index, kind: slot.kind }] : undefined,
        status: fixed ? "success" : "loading",
    };
    if (fixed)
        Object.assign(metadata, {
            storageKey: fixed.storageKey,
            content: resolveResourceUrl(fixed.storageKey),
            mimeType: fixed.mimeType,
            bytes: fixed.bytes,
            naturalWidth: fixed.width,
            naturalHeight: fixed.height,
            durationMs: fixed.durationMs,
            errorDetails: undefined,
        });
    const target = { ...node, metadata };
    const boundRoot = { ...(id === rootId ? target : root), metadata: { ...(id === rootId ? metadata : root.metadata), platformOutputRequest: task.platformGenerationRequest, platformOutputNodeIds: { ...bound, [slot.index]: id } } };
    const changed = nodes.map((n) => (n.id === rootId ? boundRoot : n.id === id ? target : n));
    if (!previous) changed.push(target);
    return { nodes: changed, target, effectKey };
}
