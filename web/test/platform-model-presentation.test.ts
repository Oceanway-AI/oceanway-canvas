import { expect, test } from "bun:test";
import {
    platformFeatureSummaries,
    platformHealthLabel,
    platformHealthTone,
    platformModelPriceLabel,
    platformPriceConditions,
    type PlatformModel,
    type PlatformModelFeature,
    type PlatformModelHealth,
    type PlatformPriceRange,
    type PlatformModelGroup,
} from "../src/lib/platform-models";
import { systemChannelModelChannels } from "../src/lib/user-session";
import { filterModelPickerGroups, groupModelsForPicker, modelPickerSeries } from "../src/lib/model-picker-groups";
import { defaultConfig, normalizeConfigSnapshot } from "../src/stores/use-config-store";
import type { PublicChannelModel } from "../src/services/api/logical-models";

const health: PlatformModelHealth = {
    scope: "platform",
    basis: "generation_outcome",
    state: "ready",
    window_start: 100,
    window_end: 200,
    sampled_at: 190,
    sample_count: 101,
    success_count: 100,
    success_rate: 99.01,
    buckets: [{ timestamp: 100, sample_count: 101, success_count: 100, success_rate: 99.01 }],
};
const ranges: PlatformPriceRange[] = [
    { rate_unit: "second", dimension: "seconds", min: "0.00000000123456789", max: "2.500000000000001", conditions: [{ model_name: "exact-a", price_version: "pv-a", specification: { resolution: "720p" }, amount: "0.00000000123456789" }] },
];

test("published ranges preserve exact decimal amounts and conditions without local repricing", () => {
    const model = { price: { status: "available" }, presentation: { price_summary: ranges } } as PlatformModel;
    expect(platformModelPriceLabel(model)).toBe("0.00000000123456789–2.500000000000001 积分/秒");
    expect(platformPriceConditions(ranges)).toContain('resolution="720p"');
    expect(platformPriceConditions(ranges)).toContain("pv-a");
    expect(platformPriceConditions([{ ...ranges[0], dimension: "cache_write_5m", rate_unit: "million_tokens" }])).toContain("缓存写入 5 分钟 0.00000000123456789 积分/百万 Token");
    expect(platformModelPriceLabel({ ...model, presentation: { ...model.presentation!, price_summary: [] }, price: { status: "available", version: "v1", rates: { price: "1" }, display_items: [{ basis: "example", amount: "10" }] } })).toBe(
        "按实际规格报价",
    );
});

test("no samples and disabled statistics never turn into zero percent or uncallable", () => {
    expect(platformHealthLabel({ ...health, state: "no_data", success_rate: null, sample_count: 0 })).toBe("暂无样本");
    expect(platformHealthLabel({ ...health, state: "disabled", success_rate: null })).toBe("统计未开启");
    expect(platformHealthLabel(undefined)).toBe("统计暂不可用");
    expect(platformHealthLabel(health)).toBe("99.01%");
    expect([null, 0, 69.9, 70, 90, 100].map(platformHealthTone)).toEqual(["empty", "failed", "failed", "warning", "good", "success"]);
});

test("reference counts remain scoped to the published protocol and variant", () => {
    const feature: PlatformModelFeature = { id: "ref", kind: "reference", label: "参考图片", protocol: "openai-video", variant_id: "first", operation: "create", modes: ["first_frame"], field: "/image", role: "first_frame", max_items: 1 };
    const groups = platformFeatureSummaries([feature, { ...feature, id: "ref2", variant_id: "multi", modes: ["reference"], max_items: 9 }]);
    expect(groups).toHaveLength(2);
    expect(groups.map((group) => group.labels)).toEqual([["参考图片 ≤1"], ["参考图片 ≤9"]]);
    expect(groups[0].scope).toContain("first_frame");
    expect(JSON.stringify(groups)).not.toContain("≤10");
});

test("host forwards authoritative group health instead of choosing the first variant", () => {
    const group: PlatformModelGroup = { key: "opaque-group", model_series: "Series", model_type: "image", logical_model: "Image", display_name: "Image", model_names: ["exact-a", "exact-b"], available_count: 2, health, price_summary: ranges };
    const models = ["exact-a", "exact-b"].map(
        (name) =>
            ({
                id: name,
                modelKey: name,
                displayName: name,
                capability: "image",
                icon: "",
                available: true,
                priceTiers: [],
                pricingMode: "platform",
                priceLabel: "",
                platform: { model_name: name, model_type: "image", model_series: "Series", logical_model: "Image", price: { status: "available", version: "p", rates: {} }, presentation: { health: { ...health, success_rate: 0 }, tags: ["特性"] } },
            }) as PublicChannelModel,
    );
    const channels = systemChannelModelChannels([{ id: "oceanway-platform", name: "OW", displayName: "OW", models, platformGroups: [group] }]);
    const normalized = normalizeConfigSnapshot({ config: { ...defaultConfig, channels } }).config;
    const grouped = groupModelsForPicker(
        normalized,
        models.map((model) => `oceanway-platform::${model.modelKey}`),
    );
    expect(grouped).toHaveLength(1);
    expect(grouped[0].key).toBe("opaque-group");
    expect(grouped[0].platform?.health.success_rate).toBe(99.01);
    expect(grouped[0].platform?.price_summary).toEqual(ranges);
    const byID = filterModelPickerGroups(normalized, grouped, "exact-b");
    expect(byID[0].models).toHaveLength(1);
    expect(byID[0].models[0].models).toEqual(["oceanway-platform::exact-b"]);
    expect(byID[0].platform?.health.success_rate).toBe(99.01);
    expect(filterModelPickerGroups(normalized, grouped, "特性")[0].models).toHaveLength(2);
    expect(filterModelPickerGroups(normalized, grouped, "series")).toHaveLength(1);
    expect(filterModelPickerGroups(normalized, grouped, "not-present")).toEqual([]);
    const absent = groupModelsForPicker({ ...defaultConfig, channels: channels.map((channel) => ({ ...channel, platformGroups: undefined })) }, ["oceanway-platform::exact-a"]);
    expect(absent[0].platformOwned).toBe(true);
    expect(absent[0].platform).toBeUndefined();
    const incomplete = channels.map((channel) => ({ ...channel, platformGroups: undefined, modelCosts: channel.modelCosts?.map((cost) => ({ ...cost, platform: { ...cost.platform!, model_series: undefined } })) }));
    const missingSeries = groupModelsForPicker(
        { ...defaultConfig, channels: incomplete },
        models.map((model) => `oceanway-platform::${model.modelKey}`),
    );
    expect(missingSeries).toHaveLength(2);
    expect(modelPickerSeries(missingSeries)).toHaveLength(2);
    const mixed = [grouped[0], { ...grouped[0], key: "other-logical", label: "Another", models: [{ key: "other", label: "other", models: ["other"] }] }];
    const series = modelPickerSeries(mixed);
    expect(series.map((g) => g.label)).toEqual(["Series"]);
    expect(series[0].models.flatMap((m) => m.models)).toEqual(["oceanway-platform::exact-a", "oceanway-platform::exact-b", "other"]);
    expect(series[0].logicalGroups.map((g) => g.key)).toEqual(["opaque-group", "other-logical"]);
    expect(modelPickerSeries(byID)[0].models).toEqual([grouped[0].models[1]]);
    expect(modelPickerSeries([grouped[0], { ...mixed[1], series: { name: "Series", modelType: "video" } }])).toHaveLength(2);
});
