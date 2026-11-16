import type { GenerationTask } from "@/services/api/task-center";

export type StudioGenerationSlot = {
    index: number;
    kind: "image" | "video";
    contentState: string;
    previewState: string;
    materialState: string;
    previewUrl?: string;
    failureCode?: string;
    recoverable?: boolean;
    resourceId?: string;
    media?: { storageKey: string; dataUrl: string; mimeType?: string; bytes?: number; width?: number; height?: number; durationMs?: number };
};

export function studioGenerationOutputs(task: GenerationTask): StudioGenerationSlot[] {
    if (!task.platformGenerationRequest || !task.resultJson) return [];
    const result = JSON.parse(task.resultJson) as { outputs?: StudioGenerationSlot[] };
    return Array.isArray(result.outputs) ? result.outputs : [];
}
