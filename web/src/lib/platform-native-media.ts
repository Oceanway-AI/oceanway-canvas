import { modelOptionName, resolveModelChannel, type AiConfig } from "@/stores/use-config-store";
import { slotRange, suppliesFor, supplyParameterIssue, supplyParameters, supplySelection, type MaterialCounts, type NativeSupply } from "./platform-media-supply";

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
    media_generation_v1?: boolean;
    workbench_parameters?: string[];
    id: string;
    gateway_protocol: string;
    defaults: Record<string, unknown>;
    specifications_schema: { properties: Record<string, NativeSchemaField>; required?: string[] };
    native_request_variant: {
        id?: string;
        protocol: string;
        method?: string;
        path: string;
        modes: string[];
        request_schema: Record<string, unknown>;
        size_presets?: NativeImageSizePreset[];
        output_size?: NativeImageOutputSize;
        supply_alternatives?: NativeSupply[];
        material_constraints?: NativeSupply["material_constraints"];
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

export function nativeCreationModeOptions(variants: NativeMediaVariant[], current: NativeMediaVariant | undefined, referenceModesOnly = false) {
    const modes = new Map<string, NativeMediaVariant>();
    for (const variant of variants) {
        const label = creationModeLabel(variant);
        if (referenceModesOnly && !variant.native_request_variant.native_reference_inputs?.some((ref) => ref.max_items !== 0)) continue;
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

function publishedMediaProfile(config: AiConfig, model: string) {
    return resolveModelChannel(config, model).modelDetails?.find((item) => item.model === modelOptionName(model))?.platform?.media_generation_profile;
}

export function platformModelDetailPending(config: AiConfig, model = config.model) {
    return resolveModelChannel(config, model).modelDetails?.find((item) => item.model === modelOptionName(model))?.platform?.state === "summary";
}

export function studioMediaGenerationVariant(config: AiConfig, model = config.model) {
    const source = publishedMediaProfile(config, model);
    const id = (model === config.model && config.nativeVariantId) || source?.default_variant;
    return typeof id === "string" && Array.isArray(source?.variants) && source.variants.some((variant: { id?: string; media_generation_v1?: boolean }) => variant.id === id && variant.media_generation_v1 === true) ? id : undefined;
}

export function nativeMediaProfile(config: AiConfig, model = config.model) {
    const source = publishedMediaProfile(config, model);
    if (!source) return undefined;
    const variants = (Array.isArray(source.variants) ? source.variants : []) as NativeMediaVariant[];
    const nativeVariants = variants.filter((v) => v.native_request_variant && v.specifications_schema?.properties);
    return nativeVariants.length ? { defaultId: typeof source.default_variant === "string" ? source.default_variant : "", variants: nativeVariants } : undefined;
}

export function nativeMediaVariant(config: AiConfig, model = config.model, id = model === config.model ? config.nativeVariantId : undefined) {
    const profile = nativeMediaProfile(config, model);
    return profile?.variants.find((v) => v.id === (id || profile.defaultId));
}

type ReferenceSlot = { role: string; min_items?: number; max_items?: number };

function imageRange(slots: ReferenceSlot[]) {
    let min = 0,
        max = 0;
    for (const slot of slots) {
        if (slot.role === "reference_image") {
            min += slot.min_items ?? 0;
            max += slot.max_items ?? Infinity;
        } else if (slot.min_items) return undefined;
    }
    return { min, max };
}

function imageRoutes(profile: NonNullable<ReturnType<typeof nativeMediaProfile>>, count: number, explicit: Record<string, unknown> = {}, mask = 0, choiceId?: string) {
    const routes = profile.variants
        .flatMap((variant) => suppliesFor(variant).map((supply) => ({ variant, supply, ...slotRange(supply.native_reference_inputs, "reference_image") })))
        .sort((a, b) => Number(b.variant.id === profile.defaultId) - Number(a.variant.id === profile.defaultId));
    const widest = routes.reduce<(typeof routes)[number] | undefined>((best, route) => (!best || route.max > best.max ? route : best), undefined);
    const possible = [...new Set(routes.filter((route) => route.min <= count && count <= route.max).map((route) => route.variant))];
    const counts = { reference_image: count, mask };
    const matches = possible.map((variant) => ({ variant, ...supplySelection(variant, explicit, counts) }));
    const eligible = matches.filter((row) => !row.issue);
    const contracts = new Set(
        eligible.map(({ variant, values }) => {
            const native = variant.native_request_variant;
            const bindings = native.native_reference_inputs.filter((slot) => counts[slot.role as keyof typeof counts] > 0).map(({ role, pointer, transport }) => [role, pointer, transport]);
            return JSON.stringify([variant.media_generation_v1 === true, variant.gateway_protocol, native.protocol, native.method, native.path, bindings, values]);
        }),
    );
    const choices = contracts.size > 1 ? eligible.map((row) => row.variant) : [];
    const chosen = choices.length ? eligible.find((row) => row.variant.id === choiceId) : eligible[0];
    const issue = !chosen && choices.length ? "当前素材和参数支持多个执行规格，请选择生成规格" : matches[0]?.issue;
    return { referenceVariant: widest?.variant, requestVariant: chosen?.variant, choices, fallback: eligible[0]?.variant ?? possible[0], issue, maxImages: widest?.max ?? 0 };
}

export function imageExecutionChoices(config: AiConfig, counts: MaterialCounts) {
    const profile = nativeMediaProfile(config);
    return profile ? imageRoutes(profile, counts.reference_image ?? 0, draftParameterObject(config.nativeParameters), counts.mask, config.nativeImageChoiceId).choices : [];
}

export function publishedImageReferenceLimit(config: AiConfig, model = config.model) {
    const platform = resolveModelChannel(config, model).modelDetails?.find((v) => v.model === modelOptionName(model))?.platform;
    if (platform?.model_type !== "image") return undefined;
    if (platform.state === "summary") {
        if (!platform.variants_complete || !platform.variants?.length || !platform.variants.every((v) => v.references_complete)) return undefined;
        return Math.max(0, ...platform.variants.map((v) => imageRange(v.references)?.max ?? 0));
    }
    const profile = nativeMediaProfile(config, model);
    return profile ? Math.max(0, ...profile.variants.flatMap((variant) => suppliesFor(variant).map((s) => slotRange(s.native_reference_inputs, "reference_image").max))) : undefined;
}

export function imageReferenceError(config: AiConfig, model: string, count: number, mask = 0) {
    const profile = nativeMediaProfile(config, model);
    if (!profile) return "";
    try {
        const sameModel = model === config.model;
        const { requestVariant, maxImages, issue } = imageRoutes(profile, count, nativeParameterObject(sameModel ? config.nativeParameters : undefined), mask, sameModel ? config.nativeImageChoiceId : undefined);
        return requestVariant ? "" : issue || (count > maxImages ? `当前图片模型最多支持 ${maxImages} 张参考图` : "当前图片模型需要符合编辑要求的参考图");
    } catch {
        return "模型参数格式无效，请恢复默认值";
    }
}

export function imageRequestConfig(config: AiConfig, count: number, mask = 0) {
    if (platformModelDetailPending(config)) throw new Error("模型参数尚未加载，请在模型选择器中加载后生成");
    if (!nativeMediaProfile(config)) return config;
    if (config.nativeVariantId && !nativeMediaVariant(config)) throw new Error("所选模型参数版本已失效，请刷新模型目录");
    const routes = imageRoutes(nativeMediaProfile(config)!, count, nativeParameterObject(config.nativeParameters), mask, config.nativeImageChoiceId);
    if (!routes.requestVariant) throw new Error(routes.issue || imageReferenceError(config, config.model, count, mask));
    return { ...config, nativeVariantId: routes.requestVariant.id };
}

export function nativeVideoComposerVariants(variants: NativeMediaVariant[], base: NativeMediaVariant, preferredId?: string) {
    const siblings = variants.filter((v) => v.gateway_protocol === base.gateway_protocol && v.native_request_variant.path === base.native_request_variant.path && v.native_request_variant.method === base.native_request_variant.method);
    const references = siblings.filter((v) => creationModeLabel(v) !== "文生视频");
    return {
        reference: references.find((v) => v.id === preferredId) ?? references.find((v) => creationModeLabel(v) === "全能参考") ?? references.find((v) => v.id === base.id) ?? references[0],
        text: siblings.find((v) => creationModeLabel(v) === "文生视频"),
    };
}

export function nativeCreationSettings(config: AiConfig, model: string, saved: Pick<AiConfig, "nativeParameters" | "nativeVariantId" | "nativeImageChoiceId"> & { nativeModeVariantId?: string }, referenceCount: number | boolean) {
    const profile = nativeMediaProfile(config, model);
    const savedVariant = profile?.variants.find((v) => v.id === saved.nativeVariantId);
    const fallback = profile?.variants.find((v) => v.id === profile.defaultId) ?? profile?.variants[0];
    const base = nativeMediaDefaults(config, model, { ...saved, nativeVariantId: savedVariant?.id ?? fallback?.id });
    const variant = nativeMediaVariant(config, model, base.nativeVariantId);
    const preferred = profile?.variants.find((v) => v.id === saved.nativeModeVariantId) ?? variant;
    let referenceVariant = variant;
    let requestVariant = variant;
    const isVideo = variant?.gateway_protocol.endsWith(".video");
    if (variant && isVideo) {
        const video = nativeVideoComposerVariants(profile?.variants ?? [], preferred ?? variant, saved.nativeModeVariantId ?? savedVariant?.id);
        referenceVariant = video.reference;
        const text = video.text;
        requestVariant = (referenceCount ? referenceVariant : text) ?? referenceVariant ?? variant;
        if (!referenceCount && text && referenceVariant) {
            const explicit = draftParameterObject(base.nativeParameters);
            if (supplySelection(text, explicit, {}).issue && !supplySelection(referenceVariant, explicit, {}).issue) requestVariant = referenceVariant;
        }
    } else if (profile) {
        const routes = imageRoutes(profile, Number(referenceCount), draftParameterObject(base.nativeParameters), 0, saved.nativeImageChoiceId);
        referenceVariant = routes.referenceVariant;
        requestVariant = routes.requestVariant ?? routes.fallback ?? referenceVariant ?? variant;
    }
    if (!requestVariant || !variant || requestVariant.id === variant.id) return { settings: base, referenceVariant };
    return { settings: { nativeVariantId: requestVariant.id, nativeParameters: base.nativeParameters }, referenceVariant };
}

export function nativeParameterChoices(field: NativeSchemaField, includeRange = true): unknown[] | undefined {
    if (Object.hasOwn(field, "const")) return [field.const];
    if (field.enum) return field.enum;
    if (!includeRange || field.type !== "integer" || typeof field.minimum !== "number" || typeof field.maximum !== "number") return undefined;
    const min = Math.ceil(field.minimum);
    const max = Math.floor(field.maximum);
    if (!Number.isSafeInteger(min) || !Number.isSafeInteger(max) || max < min || max - min > 60) return undefined;
    return Array.from({ length: max - min + 1 }, (_, i) => min + i).filter((n) => !field.multipleOf || n % Number(field.multipleOf) === 0);
}

export function nativeMediaReferenceLimits(config: AiConfig, model = config.model, variant = nativeMediaVariant(config, model)) {
    if (!variant) return undefined;
    const values = draftParameterObject(model === config.model ? config.nativeParameters : undefined);
    const all = suppliesFor(variant);
    const compatible = all.filter((supply) => !supplyParameterIssue(variant, supply, values));
    // Keep slots editable when a saved value conflicts. Exact admission still uses the whole draft.
    const supplies = compatible.length ? compatible : all;
    const capacity = (role: string) => Math.max(0, ...supplies.map((supply) => slotRange(supply.native_reference_inputs, role).max));
    return { maxImages: capacity("reference_image"), maxVideos: capacity("reference_video"), maxAudios: capacity("reference_audio"), maxFirstFrames: capacity("first_frame"), maxLastFrames: capacity("last_frame") };
}

export function nativeParameterObject(raw?: string): Record<string, unknown> {
    if (!raw) return {};
    const value: unknown = JSON.parse(raw);
    if (!value || typeof value !== "object" || Array.isArray(value)) throw new Error("原生参数必须是 JSON 对象");
    return value as Record<string, unknown>;
}

function draftParameterObject(raw?: string) {
    try {
        return nativeParameterObject(raw);
    } catch {
        return {};
    }
}

export function nativeParameterValues(variant: NativeMediaVariant, raw?: string) {
    return supplyParameters(variant, nativeParameterObject(raw));
}

export function nativeSelectionIssue(config: AiConfig, counts: MaterialCounts) {
    const variant = nativeMediaVariant(config);
    if (!variant) return "";
    const published = resolveModelChannel(config, config.model).modelDetails?.find((v) => v.model === modelOptionName(config.model))?.platform;
    try {
        if (published?.model_type === "image") {
            const selected = imageRequestConfig(config, counts.reference_image ?? 0, counts.mask);
            return supplySelection(nativeMediaVariant(selected)!, nativeParameterObject(selected.nativeParameters), counts).issue;
        }
        return supplySelection(variant, nativeParameterObject(config.nativeParameters), counts).issue;
    } catch (error) {
        return error instanceof Error && !(error instanceof SyntaxError) ? error.message : "模型参数格式无效，请恢复默认值";
    }
}

export function nativeMediaDefaults(config: AiConfig, model: string, explicit: Pick<AiConfig, "nativeParameters" | "nativeVariantId"> = {}) {
    const variant = nativeMediaVariant(config, model, explicit.nativeVariantId || "");
    if (!variant) return { nativeVariantId: explicit.nativeVariantId || "", nativeParameters: explicit.nativeParameters || "{}" };
    return { nativeVariantId: variant.id, nativeParameters: explicit.nativeParameters || "{}" };
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
