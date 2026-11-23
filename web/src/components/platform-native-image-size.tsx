import { useState } from "react";
import { Input } from "antd";
import { Select } from "@/components/ui/base/select";
import { applyNativeImagePreset, nativeImageSizeIssue, nativeImageSizePointers, resolutionLabel, selectedNativeImagePreset, usableNativeImagePresets } from "@/lib/platform-native-image-size";
import { materialsIssue, suppliesFor, supplySelection, type MaterialCounts } from "@/lib/platform-media-supply";
import { businessRoutes, publishedParameters } from "@/lib/platform-media-business";
import type { NativeImageOutputSize, NativeImageSizePreset, NativeMediaVariant } from "@/lib/platform-native-media";

const geminiSize = "/generationConfig/imageConfig/imageSize";
const geminiRatio = "/generationConfig/imageConfig/aspectRatio";

export function publishedImageSizePresets(variant: NativeMediaVariant) {
    const native = variant.native_request_variant;
    const presets = usableNativeImagePresets(variant);
    if (!native.supply_alternatives || !native.output_size) return presets;
    const supplies = suppliesFor(variant);
    return presets.filter((preset) => {
        const values = applyNativeImagePreset(variant, {}, preset.id);
        return supplies.some((supply) => !nativeImageSizeIssue({ ...variant, native_request_variant: { ...native, output_size: supply.output_size ?? native.output_size } }, values));
    });
}

export function businessImagePresets(variant: NativeMediaVariant, variants: NativeMediaVariant[], references?: MaterialCounts) {
    return businessRoutes(variants, variant, references).flatMap((route) => publishedImageSizePresets(route).map((preset) => ({ route, preset })));
}

export function setBusinessImagePreset(variant: NativeMediaVariant, target: NativeMediaVariant, explicit: Record<string, unknown>, preset: NativeImageSizePreset) {
    const sizes = nativeImageSizePointers(variant);
    const kept = Object.fromEntries(Object.entries(explicit).filter(([pointer]) => !sizes.includes(pointer)));
    return applyNativeImagePreset(target, publishedParameters(target, kept), preset.id);
}

export function imageSizeDisplayValues(variant: NativeMediaVariant, values: Record<string, unknown>, references?: MaterialCounts) {
    const suggestions = { ...values };
    for (const key of nativeImageSizePointers(variant)) {
        if (!Object.hasOwn(values, key) && variant.defaults?.[key] !== undefined) suggestions[key] = variant.defaults[key];
    }
    return supplySelection(variant, suggestions, references).issue ? values : suggestions;
}

export function nativeImageSizeGuidance(variant: NativeMediaVariant, explicit: Record<string, unknown>, references?: MaterialCounts) {
    const native = variant.native_request_variant;
    const supplies = suppliesFor(variant);
    const eligible = references ? supplies.filter((supply) => !materialsIssue(variant, supply, references)) : supplies;
    const limits = eligible.map((supply) => supply.output_size ?? native.output_size).filter((size): size is NativeImageOutputSize => Boolean(size));
    // A size error should describe the supplies that can actually carry the user's materials.
    const scoped = eligible.length !== supplies.length && eligible.length ? { ...variant, native_request_variant: { ...native, supply_alternatives: eligible } } : variant;
    const issue = !eligible.length && references && supplies.length ? materialsIssue(variant, supplies[0], references) : supplySelection(scoped, explicit, references).issue;
    const maxPixels = limits.length ? Math.max(...limits.map((size) => size.max_pixels)) : undefined;
    const alignments = new Set(limits.map((size) => size.multiple_of));
    return { issue, maxPixels, alignment: alignments.size === 1 ? limits[0].multiple_of : undefined, limits };
}

// Shared by the existing Studio generation settings: the published catalog
// supplies the choices; the app's existing controls supply light/dark styling.
export function PlatformNativeImageSize({
    variant,
    variants = [variant],
    explicit,
    values,
    references,
    onChange,
}: {
    variant: NativeMediaVariant;
    variants?: NativeMediaVariant[];
    explicit: Record<string, unknown>;
    values: Record<string, unknown>;
    references?: MaterialCounts;
    onChange: (next: Record<string, unknown>) => void;
}) {
    const native = variant.native_request_variant;
    const entries = businessImagePresets(variant, variants, references);
    const presets = entries.map((entry) => entry.preset);
    const selected = selectedNativeImagePreset(variant, values);
    const gemini = native.protocol.startsWith("gemini.");
    const limits = gemini ? undefined : native.output_size;
    const currentRatio = selected?.aspect_ratio ?? (gemini && typeof values[geminiRatio] === "string" ? values[geminiRatio] : undefined);
    const currentTier = selected?.resolution ?? (gemini && typeof values[geminiSize] === "string" ? values[geminiSize] : undefined);
    const guidance = nativeImageSizeGuidance(variant, explicit, references);
    const [custom, setCustom] = useState(false);
    const mode = !gemini && values["/size"] === "auto" ? "auto" : custom && limits?.allow_custom ? "custom" : selected ? "preset" : limits?.allow_custom ? "custom" : "preset";
    const options = [
        ...(presets.length ? [{ value: "preset", label: "预设尺寸" }] : []),
        ...(limits?.allow_custom ? [{ value: "custom", label: "自定义尺寸", disabled: !guidance.limits.some((size) => size.allow_custom) }] : []),
        ...(limits?.allow_auto ? [{ value: "auto", label: "自动尺寸", disabled: !guidance.limits.some((size) => size.allow_auto) }] : []),
    ];
    const ratios = [...new Set(presets.map((p) => p.aspect_ratio))];
    const tiers = [...new Set(presets.map((p) => p.resolution))];
    const choose = (preset: NativeImageSizePreset, axis?: "aspect_ratio" | "resolution") => {
        setCustom(false);
        const route = entries.find((item) => item.preset === preset)?.route ?? variant;
        const next = route === variant ? applyNativeImagePreset(variant, explicit, preset.id) : setBusinessImagePreset(variant, route, explicit, preset);
        if (route === variant && gemini && axis) {
            const other = axis === "resolution" ? geminiRatio : geminiSize;
            // Gemini exposes two independent pointers. Keep a previous explicit choice,
            // including a conflict, so the user can see and correct the actual draft.
            if (Object.hasOwn(explicit, other)) next[other] = explicit[other];
            else if (axis === "resolution" || currentTier === preset.resolution) delete next[other];
        }
        onChange(next);
    };
    const [width = "", height = ""] = typeof values["/size"] === "string" ? values["/size"].split("x") : [];
    return (
        <fieldset className="min-w-0 space-y-2">
            <legend className="mb-2 text-xs">输出尺寸</legend>
            {options.length > 1 ? (
                <Select
                    ariaLabel="尺寸方式"
                    className="w-full"
                    value={mode}
                    options={options}
                    onChange={(next) => {
                        setCustom(next === "custom");
                        if (next === "preset" && presets.length) choose(selected || presets[0]);
                        if (next === "auto") onChange({ ...explicit, "/size": "auto" });
                        if (next === "custom" && (!width || width === "auto")) onChange({ ...explicit, "/size": "x" });
                    }}
                />
            ) : null}
            {mode === "preset" && presets.length ? (
                <>
                    <div className="grid grid-cols-2 gap-2">
                        <Select
                            ariaLabel="分辨率"
                            className="w-full"
                            placeholder="选择分辨率"
                            value={currentTier}
                            options={tiers.map((tier) => ({ value: tier, label: resolutionLabel(tier, Boolean(limits)) }))}
                            onChange={(tier) => {
                                const p = presets.find((p) => p.resolution === tier && p.aspect_ratio === currentRatio) || presets.find((p) => p.resolution === tier);
                                if (p) choose(p, "resolution");
                            }}
                        />
                        <Select
                            ariaLabel="宽高比"
                            className="w-full"
                            placeholder="选择比例"
                            value={currentRatio}
                            options={ratios.map((ratio) => ({ value: ratio, label: ratio }))}
                            onChange={(ratio) => {
                                const p = presets.find((p) => p.aspect_ratio === ratio && p.resolution === currentTier) || presets.find((p) => p.aspect_ratio === ratio);
                                if (p) choose(p, "aspect_ratio");
                            }}
                        />
                    </div>
                    {selected?.width && selected.height ? (
                        <p className="text-xs opacity-70">
                            {selected.width} × {selected.height} px
                        </p>
                    ) : null}
                </>
            ) : null}
            {mode === "custom" && limits?.allow_custom ? (
                <div className="grid grid-cols-2 gap-2">
                    <label className="space-y-1 text-xs">
                        宽度 (px)
                        <Input aria-label="自定义宽度" inputMode="numeric" value={width === "auto" ? "" : width} onChange={(e) => onChange({ ...explicit, "/size": `${e.target.value}x${height}` })} />
                    </label>
                    <label className="space-y-1 text-xs">
                        高度 (px)
                        <Input aria-label="自定义高度" inputMode="numeric" value={height} onChange={(e) => onChange({ ...explicit, "/size": `${width}x${e.target.value}` })} />
                    </label>
                </div>
            ) : null}
            {guidance.maxPixels !== undefined ? (
                <p className="text-xs opacity-70">
                    {references ? "当前素材下" : ""}单张最多 {guidance.maxPixels.toLocaleString()} 像素。{guidance.alignment ? `宽高按 ${guidance.alignment} 对齐。` : ""}
                </p>
            ) : null}
            {guidance.issue ? (
                <p role="alert" className="text-xs">
                    {guidance.issue}
                </p>
            ) : null}
        </fieldset>
    );
}
