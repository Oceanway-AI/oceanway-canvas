export type PlatformPrice = {
    status: string;
    version: string;
    rate_unit?: string;
    rates: Record<string, string>;
    [key: string]: unknown;
};

export type PlatformModel = {
    model_name: string;
    model_type: string;
    model_series?: string;
    logical_model?: string;
    series_info?: PlatformSeriesInfo;
    display_icon?: string;
    display_vendor?: string;
    presentation?: PlatformModelPresentation;
    content?: {
        model_series?: string;
        logical_model?: string;
        display_name?: string;
        source_label?: string;
        vendor_icon?: string;
        tags?: string[];
    };
    price: PlatformPrice;
    media_generation_profile?: Record<string, unknown>;
    [key: string]: unknown;
};

export type PlatformSeriesInfo = { name: string; icon?: string; description?: string; display_order?: number };
export type PlatformModelHealthBucket = { timestamp: number; sample_count: number; success_count: number; success_rate: number | null };
export type PlatformModelHealth = {
    scope: "platform";
    basis: "generation_outcome";
    state: "ready" | "no_data" | "disabled" | "unavailable";
    window_start: number;
    window_end: number;
    sampled_at: number;
    sample_count: number;
    success_count: number;
    success_rate: number | null;
    buckets: PlatformModelHealthBucket[];
};
export type PlatformPriceRange = {
    rate_unit: string;
    dimension: string;
    min: string;
    max: string;
    conditions: { model_name: string; price_version: string; specification: Record<string, unknown>; amount: string }[];
};
export type PlatformModelFeature = {
    id: string;
    kind: "reference" | "parameter" | "output";
    label: string;
    protocol: string;
    variant_id: string;
    operation: string;
    modes: string[];
    field: string;
    role?: string;
    mime_types?: string[];
    min_items?: number;
    max_items?: number;
    values?: unknown[];
    minimum?: number;
    maximum?: number;
    unit?: string;
};
export type PlatformModelPresentation = {
    version: "model-presentation.v1";
    summary: string;
    description: string;
    tags: string[];
    features: PlatformModelFeature[];
    availability: { state: "available" | "unavailable"; reason?: string };
    price_summary: PlatformPriceRange[];
    health: PlatformModelHealth;
};
export type PlatformModelGroup = {
    key: string;
    model_series: string;
    model_type: string;
    logical_model: string;
    display_name: string;
    series_info?: PlatformSeriesInfo;
    model_names: string[];
    available_count: number;
    health: PlatformModelHealth;
    price_summary: PlatformPriceRange[];
};

const priceUnits: Record<string, string> = { million_tokens: "百万 Token", call: "次", second: "秒", count: "个", token: "Token", credit: "单位" };
const priceDimensions: Record<string, string> = { input: "输入", output: "输出", cached: "缓存", cache_read: "缓存读取", cache_write: "缓存写入", cache_write_5m: "缓存写入 5 分钟", cache_write_1h: "缓存写入 1 小时" };

function priceDimensionLabel(dimension: string) {
    return priceDimensions[dimension] || (dimension && !["seconds", "count", "call", "calls", "price", "fixed"].includes(dimension) ? dimension : "");
}

export function platformPriceRangesLabel(ranges: PlatformPriceRange[]) {
    return ranges
        .map((range) => {
            const amount = range.min === range.max ? range.min : `${range.min}–${range.max}`;
            const dimension = priceDimensionLabel(range.dimension);
            return `${dimension ? `${dimension} ` : ""}${amount} 积分/${priceUnits[range.rate_unit] || range.rate_unit}`;
        })
        .join(" · ");
}

export function platformModelPriceLabel(model: PlatformModel) {
    if (model.price.status === "unavailable") return "价格暂不可用";
    if (model.presentation) return platformPriceRangesLabel(model.presentation.price_summary) || "按实际规格报价";
    return platformPriceLabel(model.price);
}

export function platformPriceConditions(ranges: PlatformPriceRange[]) {
    return ranges
        .flatMap((range) =>
            range.conditions.map((condition) => {
                const spec =
                    Object.entries(condition.specification || {})
                        .map(([name, value]) => `${name}=${JSON.stringify(value)}`)
                        .join(", ") || "默认规格";
                const dimension = priceDimensionLabel(range.dimension);
                return `${condition.model_name} · ${spec} · ${dimension ? `${dimension} ` : ""}${condition.amount} 积分/${priceUnits[range.rate_unit] || range.rate_unit} · ${condition.price_version}`;
            }),
        )
        .join("\n");
}

// Keep each protocol/variant scope visible; mutually exclusive limits are never added.
export function platformFeatureSummaries(features: PlatformModelFeature[]) {
    const scopes = new Map<string, { key: string; scope: string; labels: string[] }>();
    for (const feature of features) {
        const key = JSON.stringify([feature.protocol, feature.variant_id, feature.operation, feature.modes]);
        let group = scopes.get(key);
        if (!group) {
            group = { key, scope: [feature.protocol, feature.operation || feature.variant_id, ...(feature.modes || [])].filter(Boolean).join(" · "), labels: [] };
            scopes.set(key, group);
        }
        let value = feature.values?.map((item) => (typeof item === "string" ? item : JSON.stringify(item))).join(" / ");
        if (feature.kind === "reference" && feature.max_items !== undefined) value = `≤${feature.max_items}`;
        else if (!value && feature.minimum !== undefined && feature.maximum !== undefined) value = `${feature.minimum}–${feature.maximum}`;
        else if (!value && feature.maximum !== undefined) value = `≤${feature.maximum}`;
        const label = [feature.label || feature.role || feature.field, value, feature.unit].filter(Boolean).join(" ");
        if (!group.labels.includes(label)) group.labels.push(label);
    }
    return [...scopes.values()];
}

export function platformHealthLabel(health?: PlatformModelHealth) {
    if (!health || health.state === "unavailable") return "统计暂不可用";
    if (health.state === "disabled") return "统计未开启";
    if (health.state === "no_data" || health.success_rate === null) return "暂无样本";
    return `${health.success_rate}%`;
}

export function platformHealthTone(rate: number | null) {
    if (rate === null || !Number.isFinite(rate)) return "empty";
    if (rate >= 100) return "success";
    if (rate >= 90) return "good";
    if (rate >= 70) return "warning";
    return "failed";
}

// Prices are already final Platform points. Keep their decimal representation;
// do not multiply again or treat missing prices as a free model.
export function platformPriceLabel(price: PlatformPrice) {
    if (price.status === "unavailable") return "价格暂不可用";
    const unit = priceUnits[price.rate_unit || ""];
    if (price.status === "usage_dependent" || !unit) return "按实际规格报价";
    const labels: Record<string, string> = { input: "输入", output: "输出", cached: "缓存", cache_read: "缓存读取", cache_write: "缓存写入", price: "", fixed: "" };
    const rates = Object.entries(price.rates || {})
        .filter(([, value]) => typeof value === "string" && /^\d+(?:\.\d+)?$/.test(value))
        .map(([name, value]) => `${labels[name] ?? name} ${value} 积分/${unit}`.trim());
    return rates.join(" · ") || "按实际规格报价";
}
