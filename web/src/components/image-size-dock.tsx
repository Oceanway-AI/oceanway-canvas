import { useState } from "react";
import { Link2, Unlink2 } from "lucide-react";
import { Input } from "antd";
import { applyNativeImagePreset, nativeImageSizeIssue, selectedNativeImagePreset } from "@/lib/platform-native-image-size";
import type { NativeMediaVariant } from "@/lib/platform-native-media";

export function ImageSizeDock({ variant, values, onChange }: { variant: NativeMediaVariant; values: Record<string, unknown>; onChange: (next: Record<string, unknown>) => void }) {
    const { size_presets: presets = [], output_size: limits } = variant.native_request_variant;
    const preset = selectedNativeImagePreset(variant, values);
    const [custom, setCustom] = useState(false);
    const [lock, setLock] = useState<number>();
    const auto = values["/size"] === "auto";
    const dimensions = String(values["/size"] ?? "").match(/^(\d*)x(\d*)$/);
    const width = dimensions?.[1] ?? String(preset?.width ?? "");
    const height = dimensions?.[2] ?? String(preset?.height ?? "");
    const isCustom = limits?.allow_custom && !auto && (custom || !preset);
    const issue = nativeImageSizeIssue(variant, values);
    const writeSize = (size: string) => onChange({ ...values, "/size": size });
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
        if (next) onChange(applyNativeImagePreset(variant, values, next.id));
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
                        <button type="button" key={v} aria-pressed={v === selected} onClick={() => pick(axis, v)}>
                            {v === "auto" ? "自动" : v === "custom" ? "自定义" : v.toUpperCase()}
                        </button>
                    ))}
                </div>
            </fieldset>
        );
    }
    return (
        <div className="grid gap-3">
            {options("aspect_ratio")}
            {options("resolution")}
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
            {isCustom && limits && (
                <small>
                    宽高按 {limits.multiple_of} 对齐 · 单张最多 {limits.max_pixels.toLocaleString()} 像素
                </small>
            )}
            {issue && <p role="alert">{issue}</p>}
        </div>
    );
}
