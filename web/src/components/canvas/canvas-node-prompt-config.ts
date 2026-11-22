// 节点生成配置的默认值与补丁：按节点类型推导生成模式，合并全局模型配置。纯函数。

import { type CanvasNodeData, type CanvasNodeMetadata, CanvasNodeType } from "@/types/canvas";
import { type AiConfig, defaultConfig } from "@/stores/use-config-store";
import type { ModelRequirements } from "@/lib/model-selection";
import { nativeDraftRouteKey, nativeMediaVariant } from "@/lib/platform-native-media";
import { buildGenerationConfig, type CanvasGenerationConfig } from "@/lib/canvas/canvas-project-generation";
import type { ImageDimensions } from "@/lib/canvas/canvas-reference-image-size";
import type { CanvasAudioSettingKey } from "./canvas-audio-settings-popover";
import type { CanvasNodeGenerationMode } from "./canvas-node-prompt-panel";

export function modeDisplayName(mode: CanvasNodeGenerationMode) {
    if (mode === "image") return "图片";
    if (mode === "video") return "视频";
    if (mode === "audio") return "音频";
    return "文本";
}

export function defaultMode(type: CanvasNodeData["type"]): CanvasNodeGenerationMode {
    return type === CanvasNodeType.Text || type === CanvasNodeType.Skill ? "text" : type === CanvasNodeType.Video ? "video" : type === CanvasNodeType.Audio ? "audio" : "image";
}

export function buildNodeConfig(globalConfig: AiConfig, node: CanvasNodeData, mode: CanvasNodeGenerationMode, requirements: ModelRequirements, referenceImage?: ImageDimensions): CanvasGenerationConfig {
    const config = buildGenerationConfig(globalConfig, node, mode, requirements, true, referenceImage);
    if (mode !== "audio") return config;
    return {
        ...config,
        audioEmotionControlMethod: node.metadata?.audioEmotionControlMethod || globalConfig.audioEmotionControlMethod || defaultConfig.audioEmotionControlMethod,
        audioEmotionRandom: node.metadata?.audioEmotionRandom || globalConfig.audioEmotionRandom || defaultConfig.audioEmotionRandom,
        audioEmotionHappy: node.metadata?.audioEmotionHappy || globalConfig.audioEmotionHappy || defaultConfig.audioEmotionHappy,
        audioEmotionAngry: node.metadata?.audioEmotionAngry || globalConfig.audioEmotionAngry || defaultConfig.audioEmotionAngry,
        audioEmotionSad: node.metadata?.audioEmotionSad || globalConfig.audioEmotionSad || defaultConfig.audioEmotionSad,
        audioEmotionAfraid: node.metadata?.audioEmotionAfraid || globalConfig.audioEmotionAfraid || defaultConfig.audioEmotionAfraid,
        audioEmotionDisgusted: node.metadata?.audioEmotionDisgusted || globalConfig.audioEmotionDisgusted || defaultConfig.audioEmotionDisgusted,
        audioEmotionMelancholic: node.metadata?.audioEmotionMelancholic || globalConfig.audioEmotionMelancholic || defaultConfig.audioEmotionMelancholic,
        audioEmotionSurprised: node.metadata?.audioEmotionSurprised || globalConfig.audioEmotionSurprised || defaultConfig.audioEmotionSurprised,
        audioEmotionCalm: node.metadata?.audioEmotionCalm || globalConfig.audioEmotionCalm || defaultConfig.audioEmotionCalm,
    };
}

export function promptPlaceholder(mode: CanvasNodeGenerationMode, hasImageContent: boolean, hasTextContent: boolean) {
    if (mode === "video") return "描述要生成的视频内容";
    if (mode === "audio") return "描述要生成的音频内容";
    if (mode === "image") return hasImageContent ? "输入新提示词，重新生成当前图片" : "描述要生成的图片内容";
    return hasTextContent ? "请输入你想要将本段文本修改成什么" : "请输入你想要生成的文本内容";
}

export function videoConfigPatch(key: keyof AiConfig, value: string) {
    if (key === "videoSeconds") return { seconds: value };
    if (key === "videoGenerateAudio") return { generateAudio: value };
    if (key === "videoWatermark") return { watermark: value };
    if (key === "videoArkPrivateAssetUpload") return { arkPrivateAssetUpload: value };
    return { [key]: value };
}

export function nativeCanvasParameterPatch(config: AiConfig, selectedMode: string | undefined, key: "nativeVariantId" | "nativeParameters", value: string, mode?: string): Partial<CanvasNodeMetadata> {
    const selected = key === "nativeVariantId" ? value : selectedMode;
    const routeKey = selected && (nativeDraftRouteKey(nativeMediaVariant(config, config.model, selected)) ?? selected);
    if (key === "nativeVariantId" && mode === "image") return { nativeImageChoiceId: routeKey, nativeVariantId: value, nativeParameters: config.nativeParameters };
    if (key === "nativeVariantId") return { nativeModeVariantId: routeKey, nativeVariantId: value, nativeParameters: config.nativeParameters };
    return { nativeModeVariantId: routeKey, nativeVariantId: config.nativeVariantId, nativeParameters: value };
}

export function audioConfigPatch(key: CanvasAudioSettingKey, value: string) {
    if (key === "audioVoice") return { audioVoice: value };
    if (key === "audioFormat") return { audioFormat: value };
    if (key === "audioSpeed") return { audioSpeed: value };
    if (key === "audioInstructions") return { audioInstructions: value };
    return { [key]: value };
}
