import { useState } from "react";
import { Link2, Unlink2 } from "lucide-react";
import { Input } from "antd";
import { applyNativeImagePreset, resolutionLabel, selectedNativeImagePreset } from "@/lib/platform-native-image-size";
import { businessImagePresets, nativeImageSizeGuidance, setBusinessImagePreset } from "./platform-native-image-size";
import type { MaterialCounts } from "@/lib/platform-media-supply";
import type { NativeMediaVariant } from "@/lib/platform-native-media";

export function ImageSizeDock({
    variant,
    variants = [variant],
    values,
    explicit = values,
    references,
    onChange,
}: {
    variant: NativeMediaVariant;
    variants?: NativeMediaVariant[];
    values: Record<string, unknown>;
    explicit?: Record<string, unknown>;
    references?: MaterialCounts;
    onChange: (next: Record<string, unknown>) => void;
}) {
    const entries = businessImagePresets(variant, variants, references);
    const presets = entries.map((entry) => entry.preset);
    const limits = variant.native_request_variant.output_size;
    const preset = selectedNativeImagePreset(variant, values);
    const [custom, setCustom] = useState(false);
    const [lock, setLock] = useState<number>();
    const auto = values["/size"] === "auto";
    const dimensions = String(values["/size"] ?? "").match(/^(\d*)x(\d*)$/);
    const width = dimensions?.[1] ?? String(preset?.width ?? "");
    const height = dimensions?.[2] ?? String(preset?.height ?? "");
    const isCustom = limits?.allow_custom && !auto && (custom || !preset);
    const guidance = nativeImageSizeGuidance(variant, explicit, references);
    const writeSize = (size: string) => onChange({ ...explicit, "/size": size });
    function pick(axis: "aspect_ratio" | "resolution", value: string) {
        setLock(undefined);
        setCustom(value === "custom");
        if (value === "custom") {
            if (auto) writeSize("x");
            return;
        }
        if (value === "auto" && axis === "aspect_ratio" && limits?.allow_auto) return writeSize("auto");
        const other = axis === "resolution" ? "aspect_ratio" : "resolution";
        const matches = presets.filter((p) => p[axis] === value);
        const next = matches.find((p) => p[other] === preset?.[other]) ?? matches[0];
        if (!next) return;
        const route = entries.find((entry) => entry.preset === next)?.route ?? variant;
        const changed = route === variant ? applyNativeImagePreset(variant, explicit, next.id) : setBusinessImagePreset(variant, route, explicit, next);
        if (route === variant && variant.native_request_variant.protocol.startsWith("gemini.")) {
            const otherPointer = axis === "resolution" ? "/generationConfig/imageConfig/aspectRatio" : "/generationConfig/imageConfig/imageSize";
            if (Object.hasOwn(explicit, otherPointer)) changed[otherPointer] = explicit[otherPointer];
            else delete changed[otherPointer];
        }
        onChange(changed);
    }
    function edit(axis: "width" | "height", value: string) {
        const paired = axis === "width" ? height : width;
        const step = limits?.multiple_of || 1;
        const pairedPixels = axis === "width" ? Number(value) / (lock || 1) : Number(value) * (lock || 1);
        const next = lock && Number(value) > 0 ? String(Math.max(step, Math.round(pairedPixels / step) * step)) : paired;
        setCustom(true);
        writeSize(axis === "width" ? value + "x" + next : next + "x" + value);
    }
    function options(axis: "aspect_ratio" | "resolution") {
        const ratio = axis === "aspect_ratio";
        const choices = new Set(presets.map((p) => p[axis]));
        if (ratio && limits?.allow_auto) choices.add("auto");
        if (!ratio && limits?.allow_custom) choices.add("custom");
        if (!choices.size) return null;
        const selected = ratio && auto ? "auto" : !ratio && isCustom ? "custom" : (preset?.[axis] ?? values[ratio ? "/generationConfig/imageConfig/aspectRatio" : "/generationConfig/imageConfig/imageSize"]);
        return (
            <fieldset>
                <legend>
                    <strong>{ratio ? "画幅比例" : "分辨率"}</strong>
                </legend>
                <div className="studio-video-choices">
                    {[...choices].map((v) => (
                        <button
                            type="button"
                            key={v}
                            aria-pressed={v === selected}
                            disabled={(v === "auto" && limits?.allow_auto && !guidance.limits.some((s) => s.allow_auto)) || (v === "custom" && !guidance.limits.some((s) => s.allow_custom))}
                            onClick={() => pick(axis, v)}
                        >
                            {v === "auto" ? "自动" : v === "custom" ? "自定义" : ratio ? v : resolutionLabel(v)}
                        </button>
                    ))}
                </div>
            </fieldset>
        );
    }
    return (
        <div className="grid gap-3">
            {options("resolution")}
            {options("aspect_ratio")}
            {(limits?.allow_custom || (preset?.width && preset.height)) && (
                <fieldset>
                    <legend>
                        <strong>尺寸（px）</strong>
                    </legend>
                    <div className="flex items-end gap-2">
                        <label className="min-w-0">
                            宽度
                            <Input type="number" aria-label="宽度" value={width} disabled={auto} readOnly={!limits?.allow_custom} min={limits?.min_width} max={limits?.max_width} step={limits?.multiple_of} onChange={(e) => edit("width", e.target.value)} />
                        </label>
                        {limits?.allow_custom ? (
                            <button type="button" aria-label="锁定宽高比" aria-pressed={Boolean(lock)} disabled={auto || !(Number(width) > 0 && Number(height) > 0)} onClick={() => setLock(lock ? undefined : Number(width) / Number(height))}>
                                {lock ? <Link2 size={16} /> : <Unlink2 size={16} />}
                            </button>
                        ) : (
                            "×"
                        )}
                        <label className="min-w-0">
                            高度
                            <Input
                                type="number"
                                aria-label="高度"
                                value={height}
                                disabled={auto}
                                readOnly={!limits?.allow_custom}
                                min={limits?.min_height}
                                max={limits?.max_height}
                                step={limits?.multiple_of}
                                onChange={(e) => edit("height", e.target.value)}
                            />
                        </label>
                    </div>
                </fieldset>
            )}
            {isCustom && guidance.maxPixels !== undefined && (
                <small>
                    {guidance.alignment && `宽高按 ${guidance.alignment} 对齐 · `}单张最多 {guidance.maxPixels.toLocaleString()} 像素
                </small>
            )}
            {guidance.issue && <p role="alert">{guidance.issue}</p>}
        </div>
    );
}
