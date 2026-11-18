import { dramaChoiceKey, dramaChoiceToModel } from "@/lib/drama/model-choice";
import { modelCompatibilityError, type ModelRequirements } from "@/lib/model-selection";
import type { DramaModelChoice, DramaModelOption, DramaSegmentRequest } from "@/services/api/drama";
import { videoResolutionComparisonKey } from "@/lib/video-generation-options";
import { type AiConfig } from "@/stores/use-config-store";

export const SEGMENT_UNSUPPORTED_HINT = "模型不支持当前规格，请调整时长、分辨率或更换模型";
const PLAIN_RESOLUTION = /^\d+p$/;

/**
 * 本段视频的生成规格；规格还不完整（时长、生成方式、分辨率或声音未知）时返回 undefined，
 * 不在前端拦截，交给服务端准入。changed 表示本段换了模型：request 里的画面与声音参数是按原模型算的，改用新模型的目录信息。
 */
export function dramaSegmentVideoRequirements(request: DramaSegmentRequest, option: DramaModelOption | undefined, changed: boolean, videoResolution?: string): ModelRequirements | undefined {
    if (!request.operation || request.seconds <= 0) return undefined;
    // 换了模型时服务端会按新模型重新换算分辨率；用横竖标签（如 768p竖）表达画幅的模型前端算不准，不判定。
    const resolutions = option?.videoResolutions || [];
    if (changed && !(resolutions.length && resolutions.every((value) => PLAIN_RESOLUTION.test(videoResolutionComparisonKey(value))))) return undefined;
    const resolution = (changed ? "" : request.resolution) || videoResolution || option?.defaultVideoResolution || "";
    const generateAudio = changed ? option?.generateAudio : request.generateAudio;
    if (!resolution || generateAudio === undefined) return undefined;
    // 首帧与随请求发送的参考图都作为图片输入，口径与服务端一致。
    const imageCount = request.references.filter((reference) => reference.sent).length + (request.firstFrameResourceId || request.firstFramePending ? 1 : 0);
    return {
        capability: "video",
        input: { textCount: 1, imageCount, videoCount: 0, audioCount: 0, characterCount: 0 },
        videoOperation: request.operation,
        videoSeconds: String(request.seconds),
        options: { vquality: resolution, videoGenerateAudio: generateAudio },
    };
}

/** 根据模型发布的能力检查本段生成参数。 */
export function dramaSegmentUnsupported(config: AiConfig, choice: DramaModelChoice, requirements: ModelRequirements | undefined) {
    if (!requirements) return false;
    const model = dramaChoiceToModel(config, "video", choice);
    if (!model) return false;
    return Boolean(modelCompatibilityError(config, model, requirements));
}

/** 目录里与当前选择对应的视频模型选项。 */
export function dramaVideoOptionFor(options: DramaModelOption[] | undefined, choice: DramaModelChoice) {
    const key = dramaChoiceKey(choice);
    return key ? options?.find((option) => dramaChoiceKey(option.choice) === key) : undefined;
}
