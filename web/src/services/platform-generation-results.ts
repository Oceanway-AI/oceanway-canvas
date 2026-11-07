import { getActiveUserScope } from "@/lib/user-scope";
import { parseBackendGenerationResult } from "@/services/api/generation-task";
import { refreshResource, resourceFileUrl } from "@/services/api/resources";
import type { GenerationTask, GenerationTaskOutput } from "@/services/api/task-center";

/** Platform has already adopted and bound each output. Recovery only validates that identity. */
export async function materializePlatformGenerationResults(task: GenerationTask, signal?: AbortSignal): Promise<GenerationTask> {
    const scope = getActiveUserScope();
    const assertCurrent = () => {
        if (signal?.aborted || scope !== getActiveUserScope()) throw new DOMException("生成结果校验已取消或账号已切换", "AbortError");
    };
    assertCurrent();
    if (task.status !== "succeeded" || task.type === "canvas_text") return task;
    const result = parseBackendGenerationResult(task);
    const mediaType = task.type === "canvas_image" ? "image" : task.type === "canvas_video" ? "video" : undefined;
    const items = mediaType === "image" ? result.images : mediaType === "video" && result.video ? [result.video] : undefined;
    if (!mediaType || !items?.length) throw new Error("平台生成结果缺少固定素材引用");
    const outputs: GenerationTaskOutput[] = await Promise.all(
        items.map(async (item, outputIndex) => {
            const id = /^resource:([^\s/:?#]+)$/.exec(item.storageKey || "")?.[1];
            if (!id) throw new Error("平台生成结果缺少固定素材引用");
            // Bypass metadata cache: deletion/ownership failure must win over a prior local effect.
            const resource = await refreshResource(id);
            assertCurrent();
            if (resource.id !== id || resource.provider !== "platform" || resource.status !== "ready" || resource.kind !== mediaType) throw new Error("平台生成素材尚未就绪或类型不匹配");
            item.dataUrl = resourceFileUrl(id);
            item.width = resource.width || item.width;
            item.height = resource.height || item.height;
            item.bytes = resource.size;
            item.mimeType = resource.mimeType;
            return { outputIndex, mediaType, providerArtifactRef: `resource:${id}`, materializedResourceId: id };
        }),
    );
    assertCurrent();
    return { ...task, resultJson: JSON.stringify(result), outputs, resultState: "READY" };
}
