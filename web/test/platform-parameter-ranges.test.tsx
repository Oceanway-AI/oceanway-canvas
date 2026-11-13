import { expect, test } from "bun:test";
import { renderToStaticMarkup } from "react-dom/server";
import { imageParameterAccepts, imageParameterRequest, type ImageParameter } from "../src/lib/image-parameters";
import { defaultModelCapabilityConfig, normalizeVideoValue, videoDurationAllowed, videoDurationOptions, type VideoCapabilityConfig } from "../src/lib/model-capabilities";
import { VideoSettingsPanel } from "../src/components/video-settings-panel";
import { canvasThemes } from "../src/lib/canvas-theme";
import { defaultConfig, createModelChannel, encodeChannelModel } from "../src/stores/use-config-store";

test("published image multipleOf is zero based, while native OAC step remains minimum based", () => {
    const field = { name: "strength", label: "强度", type: "number", min: 0.3, max: 0.9, step: 0.2, multipleOf: 0.2, default: 0.4 } as ImageParameter;
    expect(imageParameterAccepts(field, 0.4)).toBe(true);
    expect(imageParameterAccepts(field, 0.6)).toBe(true);
    expect(imageParameterAccepts(field, 0.5)).toBe(false);
    const image = { ...defaultModelCapabilityConfig().image!, parameters: [field] };
    expect(imageParameterRequest(image, { imageParameters: '{"strength":0.6}' }, true)).toEqual({ strength: 0.6 });
    expect(() => imageParameterRequest(image, { imageParameters: '{"strength":0.5}' }, true)).toThrow();
    const legacy = { name: "native", label: "原生", type: "number", min: 0.3, max: 0.9, step: 0.2 } as ImageParameter;
    expect(imageParameterAccepts(legacy, 0.5)).toBe(true);
    expect(imageParameterAccepts(legacy, 0.4)).toBe(false);
});

function video(duration: VideoCapabilityConfig["duration"]) {
    return { ...defaultModelCapabilityConfig().video!, duration };
}

test("published duration ranges align to multiples and preserve fractional defaults", () => {
    const profile = video({ selection: "range", min: 3, max: 9, step: 2, multipleOf: 2, default: 4 } as VideoCapabilityConfig["duration"]);
    expect(videoDurationOptions(profile)).toEqual([4, 6, 8]);
    expect(videoDurationAllowed(profile, 4)).toBe(true);
    expect(videoDurationAllowed(profile, 3)).toBe(false);
    expect(normalizeVideoValue(profile, { seconds: "6" }).seconds).toBe("6");
    const fractional = video({ selection: "range", min: 0.3, max: 0.9, step: 0.2, multipleOf: 0.2, default: 0.6 } as VideoCapabilityConfig["duration"]);
    expect(videoDurationOptions(fractional)).toEqual([0.4, 0.6, 0.8]);
    expect(normalizeVideoValue(fractional, {}).seconds).toBe("0.6");
    expect(normalizeVideoValue(fractional, { seconds: "0.8" }).seconds).toBe("0.8");
    expect(videoDurationAllowed(fractional, 0.6)).toBe(true);
    expect(videoDurationAllowed(fractional, 0.5)).toBe(false);
    const native = video({ selection: "range", min: 3, max: 9, step: 2, default: 5 });
    expect(videoDurationOptions(native)).toEqual([3, 5, 7, 9]);
});

test("native video settings render the published range instead of a different slider grid", () => {
    const profile = video({ selection: "range", min: 3, max: 9, step: 2, multipleOf: 2, default: 4 } as VideoCapabilityConfig["duration"]);
    const channel = createModelChannel({ id: "oceanway-platform", models: ["range-video"], modelCosts: [{ model: "range-video", protocol: "newapi", platform: {}, capabilityConfig: { version: 1, video: profile } }] });
    const config = { ...defaultConfig, channels: [channel], model: encodeChannelModel(channel.id, "range-video"), videoSeconds: "6" };
    const html = renderToStaticMarkup(<VideoSettingsPanel config={config} theme={canvasThemes.dark} onConfigChange={() => {}} />);
    const slider = html.match(/<input[^>]+type="range"[^>]*>/)?.[0];
    expect(slider).toContain('min="4"');
    expect(slider).toContain('max="8"');
    expect(slider).toContain('step="2"');
    expect(slider).toContain('value="6"');
});

test("Platform Seedance controls use published choices rather than model-name defaults", () => {
    const profile = video({ selection: "enum", values: [4, 8], default: 8 });
    profile.ratios = ["1:1"];
    profile.resolutions = ["480p"];
    profile.defaultRatio = "1:1";
    profile.defaultResolution = "480p";
    const name = "doubao-seedance-2-0-mini-260615";
    const channel = createModelChannel({ id: "oceanway-platform", models: [name], modelCosts: [{ model: name, protocol: "volcengine-ark-video", platform: {}, capabilityConfig: { version: 1, video: profile } }] });
    const config = { ...defaultConfig, channels: [channel], model: encodeChannelModel(channel.id, name), videoSeconds: "8", size: "1:1", vquality: "480p" };
    const html = renderToStaticMarkup(<VideoSettingsPanel config={config} theme={canvasThemes.dark} onConfigChange={() => {}} />);
    expect(html).not.toContain("1080P");
    expect(html).not.toContain("720P");
    expect(html).not.toContain("9:16");
    expect(html).not.toContain('type="range"');
    expect(html).not.toContain("自动同步可信素材");
    expect(html).toContain("8s");
});
