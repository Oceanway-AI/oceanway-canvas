import { modelOptionName, resolveModelChannel, type AiConfig } from "@/stores/use-config-store";

export type NativeSchemaField = Record<string, unknown> & { type?: string | string[]; enum?: unknown[]; const?: unknown; title?: string; description?: string };
export type NativeImageSizePreset = { id: string; resolution: string; aspect_ratio: string; width?: number; height?: number; values: Record<string, unknown> };
export type NativeImageOutputSize = {
    allow_custom: boolean;
    allow_auto: boolean;
    multiple_of: number;
    min_width: number;
    max_width: number;
    min_height: number;
    max_height: number;
    min_pixels: number;
    max_pixels: number;
    min_aspect_ratio: number;
    max_aspect_ratio: number;
};
export type NativeMediaVariant = {
    workbench_parameters?: string[];
    id: string;
    gateway_protocol: string;
    defaults: Record<string, unknown>;
    specifications_schema: { properties: Record<string, NativeSchemaField>; required?: string[] };
    native_request_variant: {
        id?: string;
        protocol: string;
        path: string;
        modes: string[];
        request_schema: Record<string, unknown>;
        size_presets?: NativeImageSizePreset[];
        output_size?: NativeImageOutputSize;
        native_reference_inputs: { id: string; pointer: string; role: string; transport: string; min_items?: number; max_items?: number; mime_types?: string[] }[];
    };
};

function creationModeLabel(variant: NativeMediaVariant) {
    const roles = new Set((variant.native_request_variant.native_reference_inputs || []).filter((ref) => ref.max_items !== 0).map((ref) => ref.role));
    if (!roles.size) return variant.gateway_protocol.endsWith(".video") ? "文生视频" : "文生图";
    if (roles.has("mask") || roles.has("markup")) return "图片编辑";
    if ([...roles].every((role) => role === "first_frame" || role === "last_frame")) {
        if (roles.size === 2) return "首尾帧";
        return roles.has("first_frame") ? "首帧" : "尾帧";
    }
    if (roles.has("reference_image") && roles.has("reference_video") && roles.has("reference_audio")) return "全能参考";
    if (roles.size > 1) return "多模态参考";
    if (roles.has("reference_image")) return "图片参考";
    if (roles.has("reference_video")) return "视频参考";
    if (roles.has("reference_audio")) return "音频参考";
    return "参考素材";
}

export function nativeCreationModeOptions(variants: NativeMediaVariant[], current: NativeMediaVariant | undefined, automaticVideo = false) {
    const modes = new Map<string, NativeMediaVariant>();
    for (const variant of variants) {
        const label = creationModeLabel(variant);
        if (automaticVideo && label === "文生视频") continue;
        const existing = modes.get(label);
        if (!existing || variant.id === current?.id || (existing.id !== current?.id && existing.gateway_protocol !== current?.gateway_protocol && variant.gateway_protocol === current?.gateway_protocol)) {
            modes.set(label, variant);
        }
    }
    return Array.from(modes, ([label, variant]) => ({ value: variant.id, label }));
}

export function nativeMediaVariantOptions(variants: NativeMediaVariant[]) {
    const roleLabels: Record<string, string> = { first_frame: "首帧", last_frame: "尾帧", reference_image: "参考图片", reference_video: "参考视频", reference_audio: "参考音频", mask: "蒙版" };
    const options = variants.map((variant) => {
        const refs = (variant.native_request_variant.native_reference_inputs || []).filter((ref) => ref.max_items !== 0);
        const references = refs
            .map((ref) => {
                const name = roleLabels[ref.role] || ref.role;
                const required = (ref.min_items || 0) > 0;
                const count = ref.max_items && ref.max_items > 1 ? `，最多 ${ref.max_items} 个` : "";
                return `${name}（${required ? "必填" : "可选"}${count}）`;
            })
            .join(" + ");
        const mode = references || (variant.gateway_protocol.endsWith(".video") ? "文生视频" : "无参考素材");
        return { value: variant.id, label: variant.workbench_parameters ? mode : `${variant.gateway_protocol} · ${mode}` };
    });
    return options.map((option, i) => (options.some((other, j) => j !== i && other.label === option.label) ? { ...option, label: `${option.label} · ${variants[i].native_request_variant.id || option.value}` } : option));
}

export function nativeMediaProfile(config: AiConfig, model = config.model) {
    const source = resolveModelChannel(config, model).modelCosts?.find((item) => item.model === modelOptionName(model))?.platform?.media_generation_profile;
    if (!source?.native_request_variant && !source?.gateway_protocol) return undefined;
    const variants = (Array.isArray(source.variants) ? source.variants : []) as NativeMediaVariant[];
    return { defaultId: typeof source.default_variant === "string" ? source.default_variant : "", variants: variants.filter((v) => v.native_request_variant && v.specifications_schema?.properties) };
}

export function nativeMediaVariant(config: AiConfig, model = config.model, id = model === config.model ? config.nativeVariantId : undefined) {
    const profile = nativeMediaProfile(config, model);
    return profile?.variants.find((v) => v.id === (id || profile.defaultId));
}

// A text-to-image default can accept attachments in the composer when the
// same model publishes one unambiguous edit operation for reference images.
export function nativeImageReferenceVariant(config: AiConfig, model: string, current: NativeMediaVariant | undefined) {
    if (current?.gateway_protocol !== "openai.image.generate") return current;
    const edits = nativeMediaProfile(config, model)?.variants.filter((v) => v.gateway_protocol === "openai.image.edit" && v.native_request_variant.native_reference_inputs?.some((r) => r.role === "reference_image" && r.max_items !== 0));
    return edits?.length === 1 ? edits[0] : current;
}

export function nativeVideoComposerVariants(variants: NativeMediaVariant[], base: NativeMediaVariant, preferredId?: string) {
    const siblings = variants.filter((v) => v.gateway_protocol === base.gateway_protocol && v.native_request_variant.path === base.native_request_variant.path);
    const references = siblings.filter((v) => creationModeLabel(v) !== "文生视频");
    return {
        reference: references.find((v) => v.id === preferredId) ?? references.find((v) => creationModeLabel(v) === "全能参考") ?? references.find((v) => v.id === base.id) ?? references[0],
        text: siblings.find((v) => creationModeLabel(v) === "文生视频"),
    };
}

export function nativeCreationSettings(config: AiConfig, model: string, saved: Pick<AiConfig, "nativeParameters" | "nativeVariantId">, hasReferences: boolean) {
    const profile = nativeMediaProfile(config, model);
    const savedVariant = profile?.variants.find((v) => v.id === saved.nativeVariantId);
    const fallback = profile?.variants.find((v) => v.id === profile.defaultId) ?? profile?.variants[0];
    const base = nativeMediaDefaults(config, model, savedVariant || (!saved.nativeVariantId && fallback?.id === profile?.defaultId) ? saved : { nativeVariantId: fallback?.id });
    const variant = nativeMediaVariant(config, model, base.nativeVariantId);
    let referenceVariant = nativeImageReferenceVariant(config, model, variant);
    let requestVariant = hasReferences ? referenceVariant : variant;
    if (variant?.gateway_protocol.endsWith(".video")) {
        const video = nativeVideoComposerVariants(profile?.variants ?? [], variant, savedVariant?.id);
        referenceVariant = video.reference;
        const text = video.text;
        requestVariant = (hasReferences ? referenceVariant : text) ?? referenceVariant ?? variant;
    }
    if (!requestVariant || !variant || requestVariant.id === variant.id) return { settings: base, referenceVariant };
    const values = nativeParameterValues(requestVariant);
    const previous = nativeParameterObject(base.nativeParameters);
    for (const [key, field] of Object.entries(requestVariant.specifications_schema.properties)) {
        if ((!requestVariant.workbench_parameters || requestVariant.workbench_parameters.includes(key)) && Object.hasOwn(previous, key) && JSON.stringify(field) === JSON.stringify(variant.specifications_schema.properties[key]))
            values[key] = previous[key];
    }
    return { settings: { nativeVariantId: requestVariant.id, nativeParameters: JSON.stringify(values) }, referenceVariant };
}

export function nativeMediaReferenceLimits(config: AiConfig, model = config.model, variant = nativeMediaVariant(config, model)) {
    if (!variant) return undefined;
    const refs = variant.native_request_variant.native_reference_inputs || [];
    const capacity = (roles: string[]) => refs.filter((ref) => roles.includes(ref.role)).reduce((sum, ref) => sum + (ref.max_items ?? Number.POSITIVE_INFINITY), 0);
    return { maxImages: capacity(["reference_image", "first_frame", "last_frame"]), maxVideos: capacity(["reference_video"]), maxAudios: capacity(["reference_audio"]) };
}

export function nativeParameterObject(raw?: string): Record<string, unknown> {
    if (!raw) return {};
    const value: unknown = JSON.parse(raw);
    if (!value || typeof value !== "object" || Array.isArray(value)) throw new Error("原生参数必须是 JSON 对象");
    return value as Record<string, unknown>;
}

export function nativeParameterValues(variant: NativeMediaVariant, raw?: string) {
    const values = { ...variant.defaults };
    for (const [pointer, field] of Object.entries(variant.specifications_schema.properties)) {
        if (!(pointer in values) && Object.hasOwn(field, "const")) values[pointer] = field.const;
    }
    const merged = { ...values, ...nativeParameterObject(raw) };
    if (!variant.workbench_parameters) return merged;
    return Object.fromEntries(variant.workbench_parameters.filter((key) => Object.hasOwn(merged, key)).map((key) => [key, merged[key]]));
}

export function nativeMediaDefaults(config: AiConfig, model: string, explicit: Pick<AiConfig, "nativeParameters" | "nativeVariantId"> = {}) {
    const variant = nativeMediaVariant(config, model, explicit.nativeVariantId || "");
    if (!variant) return { nativeVariantId: explicit.nativeVariantId || "", nativeParameters: explicit.nativeParameters || "{}" };
    return { nativeVariantId: variant.id, nativeParameters: variant.workbench_parameters ? JSON.stringify(nativeParameterValues(variant, explicit.nativeParameters)) : (explicit.nativeParameters ?? JSON.stringify(nativeParameterValues(variant))) };
}

export function nativeMediaSummary(config: AiConfig) {
    const variant = nativeMediaVariant(config);
    if (!variant) return "选择模型参数";
    try {
        const values = nativeParameterValues(variant, config.nativeParameters);
        return nativeParameterSummary(values, Boolean(variant.workbench_parameters));
    } catch {
        return "参数格式无效";
    }
}

export function creativeParameterLabel(pointer: string, title?: string) {
    const labels: Record<string, string> = {
        size: "尺寸",
        imageSize: "分辨率",
        resolution: "分辨率",
        quality: "画质",
        aspectRatio: "画幅",
        aspect_ratio: "画幅",
        ratio: "画幅",
        duration: "时长",
        seconds: "时长",
        generate_audio: "生成音频",
        n: "生成数量",
        output_format: "输出格式",
        output_compression: "压缩率",
        background: "背景",
        seed: "随机种子",
    };
    const leaf = pointer.split("/").at(-1) || pointer;
    return labels[leaf] || title || leaf;
}

export function nativeParameterSummary(values: Record<string, unknown>, creative = false) {
    return (
        Object.entries(values)
            .filter(([, v]) => ["string", "number", "boolean"].includes(typeof v))
            .slice(0, 3)
            .map(([k, v]) => `${creative ? creativeParameterLabel(k) : k.split("/").at(-1)} ${creative && v === "auto" ? "自动" : v}`)
            .join(" · ") || "模型参数"
    );
}
