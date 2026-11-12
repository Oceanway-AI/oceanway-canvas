import { expect, test } from "bun:test";
import { applyNativeImagePreset, nativeImageSizeIssue, nativeImageSizePointers, selectedNativeImagePreset } from "@/lib/platform-native-image-size";
import type { NativeMediaVariant } from "@/lib/platform-native-media";

export const imageSizeVariant: NativeMediaVariant = {
    id: "image",
    gateway_protocol: "openai.image.generate",
    defaults: { "/size": "1024x1024", "/n": 1 },
    specifications_schema: { properties: { "/size": { type: "string" }, "/n": { type: "integer" } } },
    native_request_variant: {
        protocol: "openai.image.generate",
        path: "/v1/images/generations",
        modes: ["sync"],
        request_schema: {},
        native_reference_inputs: [],
        size_presets: [
            { id: "square", resolution: "1K", aspect_ratio: "1:1", width: 1024, height: 1024, values: { size: "1024x1024" } },
            { id: "landscape", resolution: "2K", aspect_ratio: "16:9", width: 2560, height: 1440, values: { size: "2560x1440" } },
        ],
        output_size: { allow_custom: true, allow_auto: false, multiple_of: 16, min_width: 16, max_width: 3840, min_height: 16, max_height: 3840, min_pixels: 655360, max_pixels: 4194304, min_aspect_ratio: 1 / 3, max_aspect_ratio: 3 },
    },
};

test("image presets use exact published native values and preserve other controls", () => {
    const values = applyNativeImagePreset(imageSizeVariant, { "/size": "auto", "/n": 3, "/quality": "high" }, "landscape");
    expect(values).toEqual({ "/size": "2560x1440", "/n": 3, "/quality": "high" });
    expect(selectedNativeImagePreset(imageSizeVariant, values)?.id).toBe("landscape");
    expect(nativeImageSizePointers(imageSizeVariant)).toEqual(["/size"]);
    expect(() => applyNativeImagePreset(imageSizeVariant, values, "unpublished")).toThrow();
    // An asymmetric upstream cap is never turned into a local 2048 edge cap.
    expect(nativeImageSizeIssue(imageSizeVariant, values)).toBe("");
});

test("custom native dimensions enforce per-image pixel area, integers, alignment and explicit auto", () => {
    const check = (size: unknown, n = 1) => nativeImageSizeIssue(imageSizeVariant, { "/size": size, "/n": n });
    expect(check("2048x2048", 10)).toBe("");
    expect(check("2560x1440")).toBe("");
    expect(check("2064x2048")).toContain("像素");
    expect(check("1025x1024")).toContain("16");
    expect(check("1024.5x1024")).not.toBe("");
    expect(check("x1024")).not.toBe("");
    expect(check("01024x1024")).not.toBe("");
    expect(check("auto")).not.toBe("");
    expect(check(undefined)).not.toBe("");
    const presetOnly = { ...imageSizeVariant, native_request_variant: { ...imageSizeVariant.native_request_variant, output_size: { ...imageSizeVariant.native_request_variant.output_size!, allow_custom: false } } };
    expect(nativeImageSizeIssue(presetOnly, { "/size": "1024x1024" })).toBe("");
    expect(nativeImageSizeIssue(presetOnly, { "/size": "2048x2048" })).not.toBe("");
});

test("Gemini presets retain native enums without invented dimensions or custom controls", () => {
    const gemini: NativeMediaVariant = {
        ...imageSizeVariant,
        gateway_protocol: "gemini.image",
        specifications_schema: { properties: { "/generationConfig/imageConfig/imageSize": { type: "string" }, "/generationConfig/imageConfig/aspectRatio": { type: "string" } } },
        native_request_variant: {
            ...imageSizeVariant.native_request_variant,
            protocol: "gemini.image",
            output_size: undefined,
            size_presets: [{ id: "native-2k-16-9", resolution: "2K", aspect_ratio: "16:9", values: { generationConfig: { imageConfig: { imageSize: "2K", aspectRatio: "16:9" } } } }],
        },
    };
    const selected = applyNativeImagePreset(gemini, { "/generationConfig/seed": 0 }, "native-2k-16-9");
    expect(selected).toEqual({ "/generationConfig/seed": 0, "/generationConfig/imageConfig/imageSize": "2K", "/generationConfig/imageConfig/aspectRatio": "16:9" });
    expect(nativeImageSizeIssue(gemini, selected)).toBe("");
    expect(selectedNativeImagePreset(gemini, selected)?.width).toBeUndefined();
    expect(nativeImageSizePointers(gemini)).not.toContain("/size");
    expect(nativeImageSizeIssue(gemini, { ...selected, "/size": "1024x1024" })).toContain("imageSize");
    expect(nativeImageSizeIssue(gemini, { ...selected, "/generationConfig/imageConfig/imageSize": "4K" })).toContain("组合");
    expect(nativeImageSizeIssue(gemini, { "/generationConfig/imageConfig/imageSize": "2K" })).toBe("");
});

test("image size controls and validation never take over video presets", () => {
    const video = { ...imageSizeVariant, gateway_protocol: "modelark.video", native_request_variant: { ...imageSizeVariant.native_request_variant, protocol: "modelark.video" } };
    expect(nativeImageSizePointers(video)).toEqual([]);
    expect(nativeImageSizeIssue(video, { "/duration": 5 })).toBe("");
});
