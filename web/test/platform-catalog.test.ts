import { expect, test } from "bun:test";
import { systemChannelModelChannels } from "../src/lib/user-session";
import { requestCreditCost } from "../src/lib/model-pricing";
import { platformPriceLabel } from "../src/lib/platform-models";
import type { PublicChannelModel } from "../src/services/api/logical-models";
import { modelCapabilityConfigFor, type ModelCapabilityConfig } from "../src/lib/model-capabilities";

test("Platform catalog keeps price decimals and never invents a local zero cost", () => {
    const platform = { model_name: "exact-model", model_type: "text", price: { version: "v1", status: "available", rate_unit: "million_tokens", rates: { input: "1.23456789", output: "0" } } };
    const model = { id: "1", modelKey: "exact-model", displayName: "Example", icon: "", capability: "text", protocol: "chat-completion", priceTiers: [], available: true, pricingMode: "platform", priceLabel: "", platform } as PublicChannelModel;
    const channel = systemChannelModelChannels([{ id: "oceanway-platform", name: "OceanWay", displayName: "OceanWay", models: [model] }])[0];
    expect(channel.modelCosts![0].platform).toEqual(platform);
    expect(platformPriceLabel(platform.price)).toContain("1.23456789");
    expect(platformPriceLabel(platform.price)).toContain("输出 0");
    expect(requestCreditCost({ channelMode: "remote", model: "exact-model", modelCosts: channel.modelCosts })).toBeNull();
    expect(platformPriceLabel({ status: "usage_dependent", version: "v1", rates: {} })).toBe("按实际规格报价");
});

test("Platform controls never inherit legacy capabilities or inferred limits", () => {
    const profile: ModelCapabilityConfig = { version: 1, text: { streaming: false, contextWindowTokens: 0, maxOutputTokens: 0, references: { promptMaxChars: 0, maxImages: 0, maxImageBytes: 0, maxVideos: 0, maxVideoBytes: 0 } } };
    const config = { channels: [{ id: "oceanway-platform", models: ["exact"], modelCosts: [{ model: "exact", protocol: "chat-completion", platform: {}, capabilityConfig: profile }] }] };
    const projected = modelCapabilityConfigFor(config, "oceanway-platform::exact");
    expect(projected).toEqual(profile);
    expect(projected.image).toBeUndefined();
    expect(projected.video).toBeUndefined();
    expect(projected.text?.contextWindowTokens).toBe(0);
});
