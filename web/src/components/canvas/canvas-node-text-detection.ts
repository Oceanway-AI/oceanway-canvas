// 图片文字识别的模型选择、看图能力检查与计费说明。识别提交（use-canvas-media-tools）与弹窗说明共用，
// 保证弹窗里展示的就是实际调用的模型。
import { modelSupportsImageInspection } from "@/lib/canvas/image-to-previs-agent";
import { platformModelPriceLabel } from "@/lib/platform-models";
import { modelOptionName, resolveModelChannel, type AiConfig } from "@/stores/use-config-store";
import { translate } from "@/i18n";

export const CANVAS_TEXT_DETECTION_UNSUPPORTED = translate("canvasB.textDetection.unsupported");

export function canvasTextDetectionModel(config: Pick<AiConfig, "textModel" | "model">) {
    return config.textModel || config.model;
}

/** 文字模型未声明图片输入能力（text.references.maxImages > 0，由管理员在模型能力中开启）时返回提示文案。 */
export function canvasTextDetectionSupportError(config: AiConfig, model: string) {
    return model && !modelSupportsImageInspection(config, model) ? CANVAS_TEXT_DETECTION_UNSUPPORTED : "";
}

/** 展示 Platform 发布的模型价格说明。 */
export function canvasTextDetectionPriceLabel(config: AiConfig, model: string) {
    if (!model) return "";
    const channel = resolveModelChannel(config, model);
    const published = channel.modelDetails?.find((item) => item.model === modelOptionName(model))?.platform;
    return published ? platformModelPriceLabel(published) : "";
}
