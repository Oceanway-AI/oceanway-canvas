import type { NativeImageSizePreset, NativeMediaVariant } from "./platform-native-media";

export function isNativeImageVariant(variant: NativeMediaVariant) {
    return ["openai.image.generate", "openai.image.edit", "gemini.image", "gemini.chat.image"].includes(variant.native_request_variant.protocol);
}

// Presets are executable native values from the published catalog. A K label
// never instructs this consumer to calculate or infer a width/height pair.
function presetParameters(variant: NativeMediaVariant, preset: NativeImageSizePreset) {
    if (!isNativeImageVariant(variant)) throw new Error("当前接口不使用图片尺寸配置");
    const result: Record<string, unknown> = {};
    const allowed = variant.native_request_variant.protocol.startsWith("gemini.") ? ["/generationConfig/imageConfig/imageSize", "/generationConfig/imageConfig/aspectRatio"] : ["/size"];
    const visit = (value: unknown, pointer: string) => {
        if (allowed.includes(pointer) && Object.hasOwn(variant.specifications_schema.properties, pointer)) {
            result[pointer] = value;
        } else if (value && typeof value === "object" && !Array.isArray(value)) {
            for (const [key, child] of Object.entries(value)) visit(child, `${pointer}/${key.replace(/~/g, "~0").replace(/\//g, "~1")}`);
        } else {
            throw new Error("尺寸预设包含未发布的参数，请刷新模型目录");
        }
    };
    visit(preset.values, "");
    if (!Object.keys(result).length) throw new Error("尺寸预设缺少原生参数，请刷新模型目录");
    return result;
}

export function nativeImageSizePointers(variant: NativeMediaVariant): string[] {
    if (!isNativeImageVariant(variant)) return [];
    const pointers = new Set<string>();
    for (const preset of variant.native_request_variant.size_presets || []) {
        for (const pointer of Object.keys(presetParameters(variant, preset))) pointers.add(pointer);
    }
    if (variant.native_request_variant.output_size) pointers.add("/size");
    return [...pointers];
}

export function selectedNativeImagePreset(variant: NativeMediaVariant, values: Record<string, unknown>) {
    if (!isNativeImageVariant(variant)) return undefined;
    return variant.native_request_variant.size_presets?.find((preset) => Object.entries(presetParameters(variant, preset)).every(([key, value]) => JSON.stringify(values[key]) === JSON.stringify(value)));
}

export function applyNativeImagePreset(variant: NativeMediaVariant, values: Record<string, unknown>, id: string) {
    const preset = variant.native_request_variant.size_presets?.find((p) => p.id === id);
    if (!preset) throw new Error("尺寸预设已失效，请刷新模型目录");
    return { ...values, ...presetParameters(variant, preset) };
}

export function nativeImageSizeIssue(variant: NativeMediaVariant, values: Record<string, unknown>): string {
    if (!isNativeImageVariant(variant)) return "";
    if (variant.native_request_variant.protocol.startsWith("gemini.")) {
        if (Object.hasOwn(values, "/size")) return "Gemini 使用原生 imageSize 与 aspectRatio，不接受 size";
        const ratio = values["/generationConfig/imageConfig/aspectRatio"],
            size = values["/generationConfig/imageConfig/imageSize"];
        if (ratio !== undefined && size !== undefined && variant.native_request_variant.size_presets?.length && !selectedNativeImagePreset(variant, values)) return "当前比例与 imageSize 组合没有可用供给";
        return "";
    }
    const limits = variant.native_request_variant.output_size;
    if (!limits) return "";
    const size = values["/size"];
    if (size === "auto") return limits.allow_auto ? "" : "当前型号不支持自动尺寸，请选择明确尺寸";
    const match = typeof size === "string" ? /^([1-9]\d*)x([1-9]\d*)$/.exec(size) : null;
    if (!match) return "宽高必须是正整数，请完整填写尺寸";
    const width = Number(match[1]),
        height = Number(match[2]);
    if (!Number.isSafeInteger(width) || !Number.isSafeInteger(height)) return "宽高必须是正整数";
    if (width % limits.multiple_of || height % limits.multiple_of) return `宽高必须是 ${limits.multiple_of} 的整数倍`;
    if (width < limits.min_width || width > limits.max_width || height < limits.min_height || height > limits.max_height) return `宽度支持 ${limits.min_width}–${limits.max_width} px，高度支持 ${limits.min_height}–${limits.max_height} px`;
    const pixels = width * height;
    if (pixels < limits.min_pixels || pixels > limits.max_pixels) return `单张图片像素须为 ${limits.min_pixels.toLocaleString()}–${limits.max_pixels.toLocaleString()}，当前为 ${pixels.toLocaleString()}`;
    if (width / height < limits.min_aspect_ratio || width / height > limits.max_aspect_ratio) return "宽高比超出当前型号范围";
    if (!limits.allow_custom && !selectedNativeImagePreset(variant, values)) return "当前型号只支持列出的尺寸预设";
    // An aggregate envelope is input guidance, not routing permission. Studio
    // and Platform still validate the same complete body before preview/submit.
    return "";
}
