import { useState, type ReactNode } from "react";
import { Popover } from "antd";
import "./media-parameter-controls.css";
import { Check, Clapperboard, Clock3, Layers, Monitor, Proportions, SlidersHorizontal } from "lucide-react";
import { ImageSizeDock } from "@/components/image-size-dock";
import { imageSizeDisplayValues } from "@/components/platform-native-image-size";
import { nativeImageSizePointers, resolutionLabel, selectedNativeImagePreset } from "@/lib/platform-native-image-size";
import { imageQualityLabel } from "@/lib/image-quality";
import { NativeParameterInput, type NativeMediaSettingKey } from "@/components/platform-native-media-settings";
import {
    creativeParameterLabel,
    imageExecutionChoices,
    nativeCreationModeOptions,
    nativeMediaVariantOptions,
    nativeParameterChoices,
    nativeParameterObject,
    publishedDraftRoute,
    type NativeMediaVariant,
    type NativeSchemaField,
} from "@/lib/platform-native-media";
import { isImageOutputCountPointer, supplyControlFields, supplySelection, type MaterialCounts } from "@/lib/platform-media-supply";
import type { AiConfig } from "@/stores/use-config-store";

type Group = "生成模式" | "生成规格" | "尺寸与分辨率" | "比例" | "背景" | "画质" | "张数" | "时长" | "声音" | "更多参数";
function fieldGroup(pointer: string, field: NativeSchemaField, image: boolean): Group {
    const key = pointer.split("/").pop()?.replaceAll("_", "").toLowerCase();
    if (!image && ["ratio", "aspectratio"].includes(key || "")) return "比例";
    if (["size", "width", "height", "ratio", "aspectratio", "resolution", "videosize", "imagesize"].includes(key || "")) return "尺寸与分辨率";
    if (image) {
        if (key === "quality") return "画质";
        if (key === "background") return "背景";
        if (isImageOutputCountPointer(pointer)) return "张数";
        return "更多参数";
    }
    if (["seconds", "duration", "durationseconds"].includes(key || "")) return "时长";
    if (["audio", "generateaudio", "withaudio", "enableaudio", "audioenabled"].includes(key || "") && (field.type === "boolean" || typeof field.const === "boolean" || field.enum?.every((v) => typeof v === "boolean"))) return "声音";
    return "更多参数";
}

function display(value: unknown, group: Group) {
    if (value === undefined || value === null || value === "") return "";
    if (value === "auto" || value === "adaptive" || (group === "时长" && Number(value) === -1)) return "自动";
    if (group === "时长" && Number.isFinite(Number(value))) return String(value) + "秒";
    if (group === "张数") return String(value) + " 张";
    if (group === "画质") return imageQualityLabel(String(value));
    if (group === "背景" && value === "transparent") return "透明";
    if (group === "背景" && value === "opaque") return "不透明";
    return String(value);
}

export function MediaParameterControls({
    mode,
    config,
    variants,
    modeVariantId,
    references,
    onChange,
    canvas = false,
}: {
    mode: "image" | "video";
    config: AiConfig;
    variants: NativeMediaVariant[];
    modeVariantId?: string;
    references?: MaterialCounts;
    onChange: (key: NativeMediaSettingKey, value: string) => void;
    canvas?: boolean;
}) {
    const [open, setOpen] = useState<Group | null>(null);
    const [sizeResetRevision, setSizeResetRevision] = useState(0);
    const variant = variants.find((v) => v.id === config.nativeVariantId);
    const modeVariant = variants.find((v) => v.id === modeVariantId) ?? variant;
    const image = mode === "image";
    const modeGroup = image ? "生成规格" : "生成模式";
    const selectedModeId = image ? publishedDraftRoute(variants, config.nativeImageChoiceId)?.id : modeVariant?.id;
    const modes = image ? nativeMediaVariantOptions(imageExecutionChoices(config, references ?? {})) : nativeCreationModeOptions(variants, modeVariant, Boolean(variant));
    const modeLabel = modes.find((mode) => mode.value === selectedModeId)?.label ?? `选择${image ? "规格" : "模式"}`;
    function menu(group: Group, label: string, icon: ReactNode, content: ReactNode) {
        return (
            <Popover
                key={group}
                trigger="click"
                placement={canvas ? "topLeft" : "bottomLeft"}
                arrow={false}
                open={open === group}
                onOpenChange={(next) => setOpen((current) => (next ? group : current === group ? null : current))}
                content={
                    <div className="studio-video-menu" data-canvas-no-zoom data-canvas-wheel-scroll aria-label={group}>
                        {content}
                    </div>
                }
            >
                <button type="button" className="creation-chat-control studio-video-trigger" aria-label={group + "：" + label} aria-expanded={open === group} title={group + "：" + label}>
                    {icon}
                    <span>{image && label === "自动" && (group === "背景" || group === "画质") ? group : label}</span>
                </button>
            </Popover>
        );
    }
    const modeMenu =
        modes.length > 0 &&
        menu(
            modeGroup,
            modeLabel,
            <Clapperboard />,
            <>
                <strong>{modeGroup}</strong>
                {modes.map((option) => (
                    <button
                        className="studio-video-option"
                        key={option.value}
                        type="button"
                        aria-pressed={option.value === selectedModeId}
                        onClick={() => {
                            if (option.value !== selectedModeId) onChange("nativeVariantId", option.value);
                            setOpen(null);
                        }}
                    >
                        {option.label}
                        {option.value === selectedModeId && <Check size={15} />}
                    </button>
                ))}
            </>,
        );
    if (!variant)
        return (
            <>
                {modeMenu}
                <span role="status">{image ? "请重新选择图片模型" : "请选择有效的生成模式"}</span>
            </>
        );
    let explicit: Record<string, unknown>;
    let values: Record<string, unknown>;
    let sizePointers: string[] = [];
    try {
        explicit = nativeParameterObject(config.nativeParameters);
        values = supplySelection(variant, explicit, references).values;
        if (image) {
            values = imageSizeDisplayValues(variant, values, references);
            const pointers = nativeImageSizePointers(variant);
            if (pointers.every((key) => !variant.workbench_parameters || variant.workbench_parameters.includes(key))) sizePointers = pointers;
        }
    } catch {
        return (
            <>
                {modeMenu}
                <button type="button" className="creation-chat-control" onClick={() => onChange("nativeParameters", "{}")}>
                    参数无效，恢复默认值
                </button>
            </>
        );
    }
    const fields = supplyControlFields(variant, explicit, values, references);
    function update(key: string, value: unknown) {
        onChange("nativeParameters", JSON.stringify({ ...explicit, [key]: value }));
    }
    const parameterMenu = (group: Group, icon: ReactNode) => {
        const advanced: Group[] = ["背景", "画质", "声音", "更多参数"];
        const items = fields.filter(({ pointer, field }) => !sizePointers.includes(pointer) && (group === "更多参数" ? advanced.includes(fieldGroup(pointer, field, image)) : fieldGroup(pointer, field, image) === group));
        const sizePicker = group === "尺寸与分辨率" && sizePointers.length > 0;
        if (!items.length && !sizePicker) return null;
        const preset = sizePicker && selectedNativeImagePreset(variant, values);
        const sizeLabel = preset
            ? [resolutionLabel(preset.resolution), preset.aspect_ratio].join(" · ")
            : sizePointers
                  .map((key) => display(values[key], group))
                  .filter(Boolean)
                  .join(" · ");
        const summary = group === "更多参数" ? group : [sizePicker ? sizeLabel : "", ...items.map(({ pointer }) => display(values[pointer], group))].filter(Boolean).join(" · ") || group;
        return menu(
            group,
            summary,
            icon,
            <>
                {sizePicker && <ImageSizeDock key={sizeResetRevision} variant={variant} values={values} explicit={explicit} references={references} onChange={(next) => onChange("nativeParameters", JSON.stringify(next))} />}
                {items.map(({ pointer: key, field, allowed, unavailable }) => {
                    const label = creativeParameterLabel(key, field.title);
                    const kind = fieldGroup(key, field, image);
                    const choices = nativeParameterChoices(field, group === "时长" || group === "张数") ?? (field.type === "boolean" ? [true, false] : undefined);
                    const required = variant.specifications_schema.required?.includes(key);
                    return (
                        <section key={key}>
                            <strong>{label}</strong>
                            {choices?.length ? (
                                <div className="studio-video-choices">
                                    {(unavailable || (!required && !Object.hasOwn(field, "const"))) && (
                                        <button type="button" aria-pressed={!Object.hasOwn(explicit, key)} onClick={() => update(key, undefined)}>
                                            {unavailable ? "清除此参数" : "模型默认"}
                                        </button>
                                    )}
                                    {choices.map((value) => (
                                        <button
                                            type="button"
                                            key={JSON.stringify(value)}
                                            aria-pressed={Object.hasOwn(field, "const") || (required ? values[key] : explicit[key]) === value}
                                            disabled={Object.hasOwn(field, "const") || !allowed(value)}
                                            onClick={() => {
                                                update(key, value);
                                                if (group === "时长" || group === "画质" || group === "背景" || group === "张数") setOpen(null);
                                            }}
                                        >
                                            {typeof value === "boolean" ? (value ? "开" : "关") : display(value, kind)}
                                        </button>
                                    ))}
                                </div>
                            ) : (
                                <NativeParameterInput pointer={key} field={field} value={values[key]} required={required} choiceAllowed={allowed} onChange={(value) => update(key, value)} />
                            )}
                        </section>
                    );
                })}
                {(image || group === "更多参数") && (
                    <button
                        type="button"
                        className="justify-self-start text-xs underline"
                        onClick={() => {
                            setSizeResetRevision((revision) => revision + 1);
                            onChange("nativeParameters", "{}");
                            setOpen(null);
                        }}
                    >
                        恢复模型默认值
                    </button>
                )}
            </>,
        );
    };
    return (
        <>
            {parameterMenu("尺寸与分辨率", <Monitor />)}
            {modeMenu}
            {image && parameterMenu("张数", <Layers />)}
            {parameterMenu("时长", <Clock3 />)}
            {!image && parameterMenu("比例", <Proportions />)}
            {parameterMenu("更多参数", <SlidersHorizontal />)}
        </>
    );
}
