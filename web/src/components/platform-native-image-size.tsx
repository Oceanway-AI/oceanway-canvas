import { useState } from "react";
import { Input } from "antd";
import { Select } from "@/components/ui/base/select";
import { applyNativeImagePreset, nativeImageSizeIssue, selectedNativeImagePreset } from "@/lib/platform-native-image-size";
import type { NativeMediaVariant } from "@/lib/platform-native-media";

// Shared by the existing Studio generation settings: the published catalog
// supplies the choices; the app's existing controls supply light/dark styling.
export function PlatformNativeImageSize({ variant, values, onChange }: { variant: NativeMediaVariant; values: Record<string, unknown>; onChange: (next: Record<string, unknown>) => void }) {
    const native = variant.native_request_variant;
    const presets = native.size_presets || [];
    const selected = selectedNativeImagePreset(variant, values);
    const gemini = native.protocol.startsWith("gemini.");
    const limits = gemini ? undefined : native.output_size;
    const currentRatio = selected?.aspect_ratio ?? (gemini && typeof values["/generationConfig/imageConfig/aspectRatio"] === "string" ? values["/generationConfig/imageConfig/aspectRatio"] : undefined);
    const currentTier = selected?.resolution ?? (gemini && typeof values["/generationConfig/imageConfig/imageSize"] === "string" ? values["/generationConfig/imageConfig/imageSize"] : undefined);
    const [custom, setCustom] = useState(false);
    const mode = !gemini && values["/size"] === "auto" ? "auto" : custom && limits?.allow_custom ? "custom" : selected ? "preset" : limits?.allow_custom ? "custom" : "preset";
    const options = [...(presets.length ? [{ value: "preset", label: "预设尺寸" }] : []), ...(limits?.allow_custom ? [{ value: "custom", label: "自定义尺寸" }] : []), ...(limits?.allow_auto ? [{ value: "auto", label: "自动尺寸" }] : [])];
    const ratios = [...new Set(presets.map((p) => p.aspect_ratio))];
    const tiers = [...new Set(presets.map((p) => p.resolution))];
    const issue = nativeImageSizeIssue(variant, values);
    const choose = (id: string) => {
        setCustom(false);
        onChange(applyNativeImagePreset(variant, values, id));
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
                        if (next === "preset" && presets.length) choose((selected || presets[0]).id);
                        if (next === "auto") onChange({ ...values, "/size": "auto" });
                        if (next === "custom" && (!width || width === "auto")) onChange({ ...values, "/size": "x" });
                    }}
                />
            ) : null}
            {mode === "preset" && presets.length ? (
                <>
                    <div className="grid grid-cols-2 gap-2">
                        <Select
                            ariaLabel="宽高比"
                            className="w-full"
                            placeholder="选择比例"
                            value={currentRatio}
                            options={ratios.map((ratio) => ({ value: ratio, label: ratio }))}
                            onChange={(ratio) => {
                                const p = presets.find((p) => p.aspect_ratio === ratio && p.resolution === currentTier) || presets.find((p) => p.aspect_ratio === ratio);
                                if (p) choose(p.id);
                            }}
                        />
                        <Select
                            ariaLabel="分辨率"
                            className="w-full"
                            placeholder="选择分辨率"
                            value={currentTier}
                            options={tiers.map((tier) => ({ value: tier, label: limits ? `${tier} 级` : tier, disabled: Boolean(currentRatio && !presets.some((p) => p.resolution === tier && p.aspect_ratio === currentRatio)) }))}
                            onChange={(tier) => {
                                const p = presets.find((p) => p.resolution === tier && p.aspect_ratio === currentRatio) || presets.find((p) => p.resolution === tier);
                                if (p) choose(p.id);
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
                        <Input aria-label="自定义宽度" inputMode="numeric" value={width === "auto" ? "" : width} onChange={(e) => onChange({ ...values, "/size": `${e.target.value}x${height}` })} />
                    </label>
                    <label className="space-y-1 text-xs">
                        高度 (px)
                        <Input aria-label="自定义高度" inputMode="numeric" value={height} onChange={(e) => onChange({ ...values, "/size": `${width}x${e.target.value}` })} />
                    </label>
                </div>
            ) : null}
            {limits ? (
                <p className="text-xs opacity-70">
                    单张最多 {limits.max_pixels.toLocaleString()} 像素；宽高按 {limits.multiple_of} 对齐。
                </p>
            ) : null}
            {issue ? (
                <p role="alert" className="text-xs">
                    {issue}
                </p>
            ) : null}
        </fieldset>
    );
}
