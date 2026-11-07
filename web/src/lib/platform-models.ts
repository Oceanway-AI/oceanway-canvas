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
    price: PlatformPrice;
    media_generation_profile?: Record<string, unknown>;
    [key: string]: unknown;
};

// Prices are already final Platform points. Keep their decimal representation;
// do not multiply again or treat missing prices as a free model.
export function platformPriceLabel(price: PlatformPrice) {
    if (price.status === "unavailable") return "价格暂不可用";
    const unit = ({ million_tokens: "百万 Token", call: "次", second: "秒", count: "个", token: "Token", credit: "单位" } as Record<string, string>)[price.rate_unit || ""];
    if (price.status === "usage_dependent" || !unit) return "按实际规格报价";
    const labels: Record<string, string> = { input: "输入", output: "输出", cached: "缓存", cache_read: "缓存读取", cache_write: "缓存写入", price: "", fixed: "" };
    const rates = Object.entries(price.rates || {})
        .filter(([, value]) => typeof value === "string" && /^\d+(?:\.\d+)?$/.test(value))
        .map(([name, value]) => `${labels[name] ?? name} ${value} 积分/${unit}`.trim());
    return rates.join(" · ") || "按实际规格报价";
}
