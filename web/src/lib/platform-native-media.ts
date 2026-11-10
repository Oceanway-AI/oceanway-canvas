import { modelOptionName, resolveModelChannel, type AiConfig } from "@/stores/use-config-store";

export type NativeSchemaField = Record<string, unknown> & { type?: string | string[]; enum?: unknown[]; const?: unknown; title?: string; description?: string };
export type NativeMediaVariant = {
    id: string;
    gateway_protocol: string;
    defaults: Record<string, unknown>;
    specifications_schema: { properties: Record<string, NativeSchemaField>; required?: string[] };
    native_request_variant: {
        protocol: string;
        path: string;
        modes: string[];
        request_schema: Record<string, unknown>;
        native_reference_inputs: { id: string; pointer: string; role: string; transport: string; min_items?: number; max_items?: number; mime_types?: string[] }[];
    };
};

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

export function nativeMediaReferenceLimits(config: AiConfig, model = config.model) {
    const variant = nativeMediaVariant(config, model);
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
    return { ...values, ...nativeParameterObject(raw) };
}

export function nativeMediaDefaults(config: AiConfig, model: string, explicit: Pick<AiConfig, "nativeParameters" | "nativeVariantId"> = {}) {
    const variant = nativeMediaVariant(config, model, explicit.nativeVariantId || "");
    if (!variant) return { nativeVariantId: explicit.nativeVariantId || "", nativeParameters: explicit.nativeParameters || "{}" };
    return { nativeVariantId: variant.id, nativeParameters: explicit.nativeParameters ?? JSON.stringify(nativeParameterValues(variant)) };
}

export function nativeMediaSummary(config: AiConfig) {
    const variant = nativeMediaVariant(config);
    if (!variant) return "选择模型参数";
    try {
        const values = nativeParameterValues(variant, config.nativeParameters);
        return nativeParameterSummary(values);
    } catch {
        return "参数格式无效";
    }
}

export function nativeParameterSummary(values: Record<string, unknown>) {
    return (
        Object.entries(values)
            .filter(([, v]) => ["string", "number", "boolean"].includes(typeof v))
            .slice(0, 3)
            .map(([k, v]) => `${k.split("/").at(-1)} ${v}`)
            .join(" · ") || "模型参数"
    );
}
