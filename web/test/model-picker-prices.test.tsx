import { expect, test } from "bun:test";
import { renderToStaticMarkup } from "react-dom/server";
import { ModelLabel, ModelPicker } from "../src/components/model-picker";
import { canvasThemes } from "../src/lib/canvas-theme";
import { defaultModelCapabilityConfig } from "../src/lib/model-capabilities";
import { modelCompatibilityError, type ModelRequirements } from "../src/lib/model-selection";
import { createModelChannel, defaultConfig, normalizeConfigSnapshot } from "../src/stores/use-config-store";

function fixture() {
    const channel = createModelChannel({
        id: "comfy",
        name: "Comfy",
        scope: "system",
        models: ["h3-multi"],
        modelDetails: [
            {
                model: "h3-multi",
                displayName: "MiniMax H3",
                channelLabel: "多图一致性",
                capability: "video",
            },
        ],
    });
    return normalizeConfigSnapshot({ config: { ...defaultConfig, channels: [channel], model: "comfy::h3-multi", videoModel: "comfy::h3-multi" } }).config;
}

const requirements: ModelRequirements = { capability: "video", videoSeconds: "5", options: { vquality: "1080p" } };

test("model rows preserve multiple promotional labels and colors with their original colors", () => {
    const config = fixture();
    config.channels[0].modelDetails![0].tags = [
        { text: "限时特价", color: "purple" },
        { text: "官方1折", color: "gold" },
    ];
    for (const theme of [canvasThemes.light, canvasThemes.dark]) {
        const markup = renderToStaticMarkup(<ModelLabel config={config} model={config.model} capability="video" theme={theme} creationVariant showConfiguredModelName={false} showPrice showDescription />);
        expect(markup).toContain('data-color="purple">限时特价');
        expect(markup).toContain('data-color="gold">官方1折');
    }
    config.channels[0].modelDetails![0].tags = [];
    const markup = renderToStaticMarkup(<ModelLabel config={config} model={config.model} capability="video" theme={canvasThemes.light} showDescription />);
    expect(markup).not.toContain("限时特价");
    expect(markup).not.toContain("官方1折");
});

test("video capability tag is derived from limits and precedes editable model tags", () => {
    const config = fixture();
    const cost = config.channels[0].modelDetails![0];
    cost.capabilityConfig = defaultModelCapabilityConfig();
    Object.assign(cost.capabilityConfig.video!.references, { maxImages: 30, maxVideos: 10, maxAudios: 10 });
    cost.tags = [
        { text: "限时特价", color: "purple" },
        { text: "按次收费", color: "blue" },
    ];

    const markup = renderToStaticMarkup(<ModelLabel config={config} model={config.model} capability="video" theme={canvasThemes.light} creationVariant showConfiguredModelName={false} showPrice showDescription />);

    expect(markup).toContain('data-color="green">图×30 · 视×10 · 音×10');
    expect(markup.indexOf("图×30 · 视×10 · 音×10")).toBeLessThan(markup.indexOf("限时特价"));
    expect(markup.indexOf("限时特价")).toBeLessThan(markup.indexOf("按次收费"));
});

test("video capability tag omits zero limits while preserving editable model tags", () => {
    const config = fixture();
    const cost = config.channels[0].modelDetails![0];
    cost.capabilityConfig = defaultModelCapabilityConfig();
    Object.assign(cost.capabilityConfig.video!.references, { maxImages: 0, maxVideos: 0, maxAudios: 0 });
    cost.tags = [{ text: "按次收费", color: "blue" }];

    const markup = renderToStaticMarkup(<ModelLabel config={config} model={config.model} capability="video" theme={canvasThemes.light} creationVariant showConfiguredModelName={false} showPrice showDescription />);

    expect(markup).not.toContain("图×");
    expect(markup).not.toContain("视×");
    expect(markup).not.toContain("音×");
    expect(markup).toContain('data-color="blue">按次收费');
});

function textFixture() {
    const channel = createModelChannel({
        id: "qwen",
        name: "Qwen",
        scope: "system",
        models: ["qwen3.8-max"],
        modelDetails: [{ model: "qwen3.8-max", displayName: "Qwen3.8-Max-0902", capability: "text" }],
    });
    return normalizeConfigSnapshot({ config: { ...defaultConfig, channels: [channel], model: "qwen::qwen3.8-max", textModel: "qwen::qwen3.8-max" } }).config;
}

test("text models show how many images and videos they can read, like video reference limits", () => {
    const config = textFixture();
    const cost = config.channels[0].modelDetails![0];
    cost.capabilityConfig = defaultModelCapabilityConfig();
    Object.assign(cost.capabilityConfig.text!.references, { maxImages: 9, maxVideos: 3 });
    cost.tags = [{ text: "按次收费", color: "blue" }];
    const markup = renderToStaticMarkup(<ModelLabel config={config} model={config.model} capability="text" theme={canvasThemes.light} creationVariant showConfiguredModelName={false} showPrice showDescription />);
    expect(markup).toContain('data-color="green">图×9 · 视×3');
    expect(markup).not.toContain("音×");
    expect(markup.indexOf("图×9 · 视×3")).toBeLessThan(markup.indexOf("按次收费"));
});

test("text models without video understanding only show the image limit, and none when vision is off", () => {
    const config = textFixture();
    const cost = config.channels[0].modelDetails![0];
    cost.capabilityConfig = defaultModelCapabilityConfig();
    Object.assign(cost.capabilityConfig.text!.references, { maxImages: 9, maxVideos: 0 });
    const render = () => renderToStaticMarkup(<ModelLabel config={config} model={config.model} capability="text" theme={canvasThemes.dark} creationVariant showConfiguredModelName={false} showPrice={false} showDescription />);
    expect(render()).toContain('data-color="green">图×9<');
    expect(render()).not.toContain("视×");
    Object.assign(cost.capabilityConfig.text!.references, { maxImages: 0, maxVideos: 0 });
    expect(render()).not.toContain("图×");
});

function platformMediaFixture(capability: "text" | "image" | "video" = "video") {
    const config = fixture();
    const cost = config.channels[0].modelDetails![0];
    cost.available = true;
    cost.capability = capability;
    cost.platform = {
        model_name: cost.model,
        model_type: capability,
        logical_model: "同一模型",
        content: { display_name: "Seedance 2.0 特惠 按条计费", source_label: "内部来源简称", tags: ["图×3", "官方长期稳定"] },
        price: { status: "available", version: "p", rates: {} },
        presentation: {
            version: "model-presentation.v1",
            summary: "内部测试说明",
            description: "技术细节",
            tags: ["图×3", "官方长期稳定"],
            features: [{ id: "f", kind: "parameter", label: "resolution", protocol: "sora.video", variant_id: "v", operation: "create", modes: ["async"], field: "/resolution" }],
            availability: { state: "available" },
            price_summary: [],
            health: { scope: "platform", basis: "generation_outcome", state: "no_data", window_start: 0, window_end: 0, sampled_at: 0, sample_count: 0, success_count: 0, success_rate: null, buckets: [] },
        },
    };
    const render = (model = config.model, requirements?: ModelRequirements) =>
        renderToStaticMarkup(<ModelLabel config={config} model={model} requirements={requirements} capability={capability} theme={canvasThemes.light} creationVariant showConfiguredModelName={false} showPrice showDescription />);
    return { channel: config.channels[0], cost, render };
}

test("Seedance picker keeps the published output token rate visible with complete or missing reference durations", () => {
    const { cost, render } = platformMediaFixture();
    const platform = cost.platform!;
    platform.media_generation_profile = {
        defaults: { "/metadata/resolution": "720p", "/metadata/aspect_ratio": "16:9", "/duration": 5 },
        native_request_profile: { capabilities: { template_id: "seedance-2.5" } },
    };
    platform.presentation!.price_summary = [
        {
            rate_unit: "million_tokens",
            dimension: "completion_tokens",
            min: "4200",
            max: "4200",
            conditions: [{ model_name: platform.model_name, price_version: platform.price.version, specification: { resolution: "720p", video_input: "video" }, amount: "4200" }],
        },
    ];
    const input = { videoCount: 2, textCount: 1, imageCount: 0, audioCount: 0, characterCount: 0 };
    for (const videoDurationsMs of [[1500, 2500], [1500]]) {
        const html = render(undefined, { capability: "video", input: { ...input, videoDurationsMs } });
        expect(html).toContain(">✦ 4200/百万输出 Token</span>");
        expect(html).toContain("720p 有参考视频 4200 OceanWay 积分/百万输出 Token");
        expect(html).not.toContain("163.296/s");
    }
});

test("platform media options show the exact display name and call ID with only curated tags", () => {
    const { cost, render } = platformMediaFixture();
    const displayName = "Seedance 2.0 特惠 按条计费";
    const html = render();
    expect(html).toContain(displayName);
    expect(html.indexOf(displayName)).toBeLessThan(html.indexOf("<code"));
    expect(html).toContain(">" + cost.model + "</code>");
    expect(html).toContain("官方长期稳定");
    expect(html).toContain('data-color="green" data-callable="true">可调用</span>');
    for (const detail of ["暂无样本", "platform-health-track", "0%", "sora.video", "resolution", "内部测试说明", "技术细节", "内部来源简称", "另有"]) expect(html).not.toContain(detail);
    cost.available = false;
    expect(render()).toContain('data-callable="false">不可调用</span>');
    cost.platform!.presentation!.tags = [];
    expect(render()).not.toContain("模型标签");
});

test.each(["image", "video"] as const)("%s catalog rows show ordinary image capacity across variants, even when a different role is required", (kind) => {
    const { cost, render } = platformMediaFixture(kind);
    const platform = cost.platform!;
    platform.state = "summary";
    platform.presentation!.tags = ["参考图0张", "官方长期稳定"];
    const slot = (role: string, max: number) => ({ role, min_items: 1, max_items: max, mime_types: [] });
    platform.variants = [
        { id: "default", references_complete: true, references: [] },
        { id: "edit", references_complete: true, references: [slot("reference_image", 3), slot(kind === "video" ? "last_frame" : "mask", 1)] },
    ];
    platform.variants_complete = true;
    expect(render()).toContain("参考图最多 3 张");
    expect(render()).not.toContain("参考图0张");
    platform.variants[1].references_complete = false;
    expect(render()).not.toContain("参考图");
    platform.variants = [{ id: "frames", references_complete: true, references: [slot("first_frame", 1), slot("last_frame", 1)] }];
    const frameRow = render();
    if (kind === "video") {
        expect(frameRow).toContain("首帧");
        expect(frameRow).toContain("尾帧");
        expect(frameRow).not.toContain("参考图 0 张");
    } else {
        expect(frameRow).toContain("参考图 0 张");
    }
});

test.each(["text", "image", "video"] as const)("%s variants under the same model keep separate callability and recent results", (capability) => {
    const { channel, cost, render } = platformMediaFixture(capability);
    const sibling = structuredClone(cost);
    sibling.model = "source-b";
    sibling.platform!.model_name = sibling.model;
    channel.models.push(sibling.model);
    channel.modelDetails!.push(sibling);
    for (const [variant, successes] of [
        [cost, 0],
        [sibling, 9],
    ] as const) {
        const bucket = { timestamp: 3600, sample_count: 10, success_count: successes, success_rate: successes * 10 };
        Object.assign(variant.platform!.presentation!.health, { state: "ready", sample_count: bucket.sample_count, success_count: successes, success_rate: bucket.success_rate, buckets: [bucket] });
    }
    sibling.platform!.presentation!.availability.state = "unavailable";
    const a = render();
    const b = render(channel.id + "::" + sibling.model);
    expect(a).toContain("可调用；近 24 小时生成成功率：0%");
    expect(a).toContain("is-failed");
    expect(a).not.toContain("90%");
    expect(b).toContain("不可调用；近 24 小时生成成功率：90%");
    expect(b).toContain("is-good");
    expect(b).not.toContain("is-failed");
    expect(a).not.toContain("data-callable");
    expect(b).not.toContain("data-callable");
});

test("text rows share the media layout and only expose input prices before hover", () => {
    const { cost, render } = platformMediaFixture("text");
    cost.platform!.presentation!.price_summary = ["input", "output", "cache_read", "cache_write_5m"].map((dimension, index) => ({
        dimension,
        rate_unit: "million_tokens",
        min: String(index),
        max: String(index),
        conditions: [{ model_name: "internal-model", price_version: "private-price-version", specification: {}, amount: String(index) }],
    }));
    const html = render();
    expect(html).toContain("canvas-model-picker-media-row");
    expect(html).toContain(">输入 ✦ 0/百万 Token</span>");
    expect(html).not.toContain(">缓存读取 ✦ 2/百万 Token</span>");
    expect(html).not.toContain("private-price-version");
    expect(html).not.toContain("默认规格");
    expect(html).toContain("输入 0 OceanWay 积分/百万 Token");
    expect(html).toContain("输出 1 OceanWay 积分/百万 Token");
    expect(html).toContain("缓存读取 2 OceanWay 积分/百万 Token");
    expect(html).not.toContain("内部测试说明");
    expect(html).not.toContain("技术细节");
    expect(html).toContain('data-callable="true"');
    cost.platform!.presentation!.price_summary = cost.platform!.presentation!.price_summary.filter((range) => range.dimension !== "input");
    expect(render()).toContain("输入价未提供");
});

test("text summaries with no media variants allow plain text before detail has been loaded", () => {
    const config = textFixture();
    const cost = config.channels[0].modelDetails![0];
    cost.platform = {
        state: "summary",
        model_name: cost.model,
        model_type: "text",
        price: { status: "available", version: "p1", rates: {} },
        variants_complete: true,
        variants: [],
    };
    const input = { textCount: 1, imageCount: 0, videoCount: 0, audioCount: 0, characterCount: 0 };
    expect(modelCompatibilityError(config, config.model, { capability: "text", input })).toBe("");
    for (const key of ["imageCount", "videoCount", "audioCount", "characterCount"] as const) {
        expect(modelCompatibilityError(config, config.model, { capability: "text", input: { ...input, [key]: 1 } })).not.toBe("");
    }
    expect(modelCompatibilityError(config, config.model, { capability: "video", input })).toBe("模型与创作类型不匹配");
    cost.platform.model_type = "video";
    expect(modelCompatibilityError(config, config.model, { capability: "video", input })).not.toBe("");
});
