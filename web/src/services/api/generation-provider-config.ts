import { normalizeAudioFormatForConfig, normalizeAudioVoiceForConfig } from "@/lib/audio-generation";
import { imageParameterRequest } from "@/lib/image-parameters";
import { nativeMediaProfile, nativeMediaVariant, nativeParameterObject } from "@/lib/platform-native-media";
import { supplySelection, type MaterialCounts } from "@/lib/platform-media-supply";
import { modelCapabilityConfigFor, videoResolutionRequest } from "@/lib/model-capabilities";
import { resolveGenerationWorkflowExecution, type GenerationWorkflowExecution } from "@/lib/generation-workflow-execution";
import { logicalModelIDForConfig, resolveModelRequestConfig, type AiConfig } from "@/stores/use-config-store";
import type { BackendGenerationMode } from "./generation-task";

export function backendProviderConfig(config: AiConfig, mode: BackendGenerationMode = "image", references?: MaterialCounts) {
    const requestConfig = resolveModelRequestConfig(config, config.model);
    if ((mode === "image" || mode === "video") && requestConfig.channelId === "oceanway-platform" && nativeMediaProfile(config)) {
        const variant = nativeMediaVariant(config);
        if (!variant) throw new Error("所选模型参数版本已失效，请刷新模型目录");
        const { values: nativeParameters, issue } = supplySelection(variant, nativeParameterObject(config.nativeParameters), references);
        if (issue) throw new Error(issue);
        return { channelId: requestConfig.channelId, model: requestConfig.model, nativeVariantId: variant.id, nativeParameters };
    }
    const workflow = resolveGenerationWorkflowExecution(config, mode);
    if (workflow) return workflowProviderConfig(config, requestConfig, workflow);
    const generationOptions = {
        ...(mode === "image" ? { imageParameters: imageParameterRequest(modelCapabilityConfigFor(config, config.model).image!, config, true) } : {}),
        size: config.size,
        imageResolution: omittedImageQuality(config.imageResolution),
        quality: omittedImageQuality(config.quality),
        transparentBackground: config.transparentBackground,
        count: config.count,
        videoSeconds: config.videoSeconds,
        vquality: config.vquality,
        videoGenerateAudio: config.videoGenerateAudio,
        videoWatermark: config.videoWatermark,
        videoArkPrivateAssetUpload: config.videoArkPrivateAssetUpload,
        audioVoice: normalizeAudioVoiceForConfig(config, config.audioVoice),
        audioFormat: normalizeAudioFormatForConfig(config, config.audioFormat),
        audioSpeed: config.audioSpeed,
        audioLanguage: config.audioLanguage,
        audioDialect: config.audioDialect,
        audioInstructions: config.audioInstructions,
        systemPrompt: config.systemPrompt,
    };
    if (logicalModelIDForConfig(config)) return generationOptions;
    if (requestConfig.channelId === "oceanway-platform") {
        const video = mode === "video" ? modelCapabilityConfigFor(config, config.model).video : undefined;
        // UI preferences may use 720; Platform validates the published value
        // (e.g. 720p). Keep unsupported input intact so admission rejects it.
        return { channelId: requestConfig.channelId, model: requestConfig.model, ...generationOptions, ...(video ? { vquality: videoResolutionRequest(video, config.vquality) ?? config.vquality } : {}) };
    }
    return {
        channelId: requestConfig.channelId,
        apiFormat: requestConfig.apiFormat,
        interfaceType: requestConfig.interfaceType,
        baseUrl: requestConfig.baseUrl,
        apiKey: requestConfig.apiKey,
        secretKey: requestConfig.secretKey,
        model: requestConfig.model,
        ...generationOptions,
        capabilityConfig: modelCapabilityConfigFor(config, requestConfig.model),
        systemPrompt: config.systemPrompt,
    };
}

function workflowProviderConfig(config: AiConfig, requestConfig: ReturnType<typeof resolveModelRequestConfig>, workflow: GenerationWorkflowExecution) {
    const runningHubActive = workflow.provider === "runninghub";
    return {
        channelId: "",
        apiFormat: requestConfig.apiFormat,
        interfaceType: workflow.interfaceType,
        baseUrl: config.runningHub.baseUrl,
        apiKey: runningHubActive ? config.runningHub.apiKey : "",
        // 工作流是独立 Provider，不能继承普通模型渠道的密钥和自定义头。
        secretKey: "",
        headers: [],
        model: workflow.providerModel,
        size: config.size,
        quality: config.quality,
        transparentBackground: config.transparentBackground,
        count: config.count,
        videoSeconds: config.videoSeconds,
        vquality: config.vquality,
        videoGenerateAudio: config.videoGenerateAudio,
        videoWatermark: config.videoWatermark,
        videoArkPrivateAssetUpload: config.videoArkPrivateAssetUpload,
        audioVoice: normalizeAudioVoiceForConfig(config, config.audioVoice),
        audioFormat: normalizeAudioFormatForConfig(config, config.audioFormat),
        audioSpeed: config.audioSpeed,
        audioLanguage: config.audioLanguage,
        audioDialect: config.audioDialect,
        audioInstructions: config.audioInstructions,
        workflowId: workflow.workflowId,
        webappId: workflow.webappId,
        workflowJson: workflow.workflowJson,
        workflowFields: workflow.workflowFields,
        runningHubUseWallet: false,
        runningHubWalletApiKey: "",
        runningHubUploadApiKey: runningHubActive ? config.runningHub.uploadApiKey || "" : "",
        capabilityConfig: modelCapabilityConfigFor(config, requestConfig.model),
        systemPrompt: config.systemPrompt,
    };
}

export function omittedImageQuality(value: string | undefined) {
    const normalized = String(value || "")
        .trim()
        .toLowerCase();
    return normalized === "auto" || normalized === "any" ? undefined : value;
}
