import { describe, expect, test } from "bun:test";
import { renderToStaticMarkup } from "react-dom/server";
import { ImageSizePicker } from "../src/components/image-size-picker";
import { IMAGE_RATIOS, imagePresetForRatio, imageSizeConfigWithPresets, imageTierAvailable } from "../src/lib/image-size-presets";
import { defaultImageCapabilityConfig } from "../src/lib/model-capabilities";
import { projectLogicalCapability } from "../src/lib/user-session";

function publishedSpec() {
    const profile = defaultImageCapabilityConfig("gemini-image", "test");
    profile.quality = { supported: false, values: [], default: "auto" };
    profile.size = imageSizeConfigWithPresets(
        profile,
        IMAGE_RATIOS.flatMap((ratio) => [imagePresetForRatio("1k", ratio), imagePresetForRatio("4k", ratio)]),
    );
    return { version: 1 as const, capability: "image" as const, imageSize: { parameter: profile.size.parameter, presets: profile.size.presets } };
}

describe("前台模型同步系统模型尺寸档位", () => {
    test("供应线路规格保留 aspect_ratio 与 1K/4K 预设", () => {
        const spec = publishedSpec();
        expect(spec?.imageSize?.parameter).toBe("aspect_ratio");
        expect(spec?.imageSize?.presets?.some((preset) => preset.tier === "1k")).toBe(true);
        expect(spec?.imageSize?.presets?.some((preset) => preset.tier === "4k")).toBe(true);
        expect(spec?.imageSize?.presets?.some((preset) => preset.tier === "2k")).toBe(false);
    });

    test("前台投影不再把多档压成只有 1K", () => {
        const spec = publishedSpec()!;
        const projected = projectLogicalCapability(spec, { size: "16:9" }).image!;
        expect(projected.size.parameter).toBe("aspect_ratio");
        expect(imageTierAvailable(projected, "1k")).toBe(true);
        expect(imageTierAvailable(projected, "4k")).toBe(true);
        expect(imageTierAvailable(projected, "2k")).toBe(false);
        const picker = renderToStaticMarkup(<ImageSizePicker profile={projected} size="16:9" quality="4k" onChange={() => {}} />);
        expect(picker).toMatch(/<button[^>]*>1K/);
        expect(picker).toMatch(/<button[^>]*>4K/);
        expect(picker).not.toMatch(/<button[^>]*>2K/);
        expect(picker).toMatch(/aria-pressed="true"[^>]*>4K/);
    });
});
