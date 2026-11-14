import { useState, type ReactNode } from "react";
import { Popover } from "antd";
import { Check, Clapperboard, Clock3, Image as ImageIcon, Layers, Monitor, SlidersHorizontal, Sparkles, Volume2, VolumeX } from "lucide-react";
import { PlatformNativeImageSize } from "@/components/platform-native-image-size";
import { nativeImageSizePointers, selectedNativeImagePreset } from "@/lib/platform-native-image-size";
import { imageQualityLabel } from "@/lib/image-quality";
import { NativeParameterInput, type NativeMediaSettingKey } from "@/components/platform-native-media-settings";
import { creativeParameterLabel, nativeCreationModeOptions, nativeMediaVariantOptions, nativeParameterValues, type NativeMediaVariant, type NativeSchemaField } from "@/lib/platform-native-media";
import type { AiConfig } from "@/stores/use-config-store";

type Group = "生成模式" | "尺寸与分辨率" | "画质" | "张数" | "时长" | "声音" | "更多参数";
type Field = [string, NativeSchemaField];

function fieldGroup([pointer, field]: Field, image: boolean): Group {
    const key = pointer.split("/").pop()?.replaceAll("_", "").toLowerCase();
    if (["size", "width", "height", "ratio", "aspectratio", "resolution", "videosize", "imagesize"].includes(key || "")) return "尺寸与分辨率";
    if (image) {
        if (key === "quality") return "画质";
        if (["n", "candidatecount", "numimages", "numoutputs"].includes(key || "")) return "张数";
        return "更多参数";
    }
    if (["seconds", "duration", "durationseconds"].includes(key || "")) return "时长";
    if (["audio", "generateaudio", "withaudio"].includes(key || "") && (field.type === "boolean" || field.enum?.every((v) => typeof v === "boolean"))) return "声音";
    return "更多参数";
}

function display(value: unknown, group: Group) {
    if (value === undefined || value === null || value === "") return "";
    if (value === "auto" || value === "adaptive" || (group === "时长" && Number(value) === -1)) return "自动";
    if (group === "时长" && Number.isFinite(Number(value))) return String(value) + "秒";
    if (group === "张数") return String(value) + " 张";
    if (group === "画质") return imageQualityLabel(String(value));
    return String(value);
}

export function MediaParameterControls({ mode, config, variants, onChange }: { mode: "image" | "video"; config: AiConfig; variants: NativeMediaVariant[]; onChange: (key: NativeMediaSettingKey, value: string) => void }) {
    const [open, setOpen] = useState<Group | null>(null);
    const [sizeResetRevision, setSizeResetRevision] = useState(0);
    const variant = variants.find((v) => v.id === config.nativeVariantId);
    const image = mode === "image";
    const modes = image && variant && !variant.workbench_parameters ? nativeMediaVariantOptions(variants) : nativeCreationModeOptions(variants, variant, !image);
    const automaticImageMode = image && variant?.workbench_parameters && variants.every((v) => ["openai.image.generate", "openai.image.edit"].includes(v.gateway_protocol));
    const modeLabel = modes.find((mode) => mode.value === variant?.id)?.label ?? "选择模式";
    function menu(group: Group, label: string, icon: ReactNode, content: ReactNode) {
        return (
            <Popover
                key={group}
                trigger="click"
                placement="bottomLeft"
                arrow={false}
                open={open === group}
                onOpenChange={(next) => setOpen((current) => (next ? group : current === group ? null : current))}
                content={
                    <div className="studio-video-menu" aria-label={group}>
                        {content}
                    </div>
                }
            >
                <button type="button" className="creation-chat-control studio-video-trigger" aria-label={group + "：" + label} aria-expanded={open === group} title={group + "：" + label}>
                    {icon}
                    <span>{label}</span>
                </button>
            </Popover>
        );
    }
    const modeMenu =
        modes.length > 0 &&
        menu(
            "生成模式",
            modeLabel,
            image ? <ImageIcon /> : <Clapperboard />,
            <>
                <strong>生成模式</strong>
                {automaticImageMode && <p className="text-xs">根据参考图自动选择模式</p>}
                {modes.map((option) => (
                    <button
                        className="studio-video-option"
                        key={option.value}
                        type="button"
                        aria-pressed={option.value === variant?.id}
                        disabled={Boolean(automaticImageMode)}
                        onClick={() => {
                            if (option.value !== variant?.id) onChange("nativeVariantId", option.value);
                            setOpen(null);
                        }}
                    >
                        {option.label}
                        {option.value === variant?.id && <Check size={15} />}
                    </button>
                ))}
            </>,
        );
    if (!variant)
        return (
            <>
                {modeMenu}
                <span role="status">请选择有效的生成模式</span>
            </>
        );
    let values: Record<string, unknown>;
    let sizePointers: string[] = [];
    try {
        values = nativeParameterValues(variant, config.nativeParameters);
        if (image) {
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
    const fields = Object.entries(variant.specifications_schema.properties).filter(([key]) => !variant.workbench_parameters || variant.workbench_parameters.includes(key));
    function update(key: string, value: unknown) {
        onChange("nativeParameters", JSON.stringify({ ...values, [key]: value }));
    }
    const parameterMenu = (group: Group, icon: ReactNode) => {
        const items = fields.filter((entry) => !sizePointers.includes(entry[0]) && fieldGroup(entry, image) === group);
        const sizePicker = group === "尺寸与分辨率" && sizePointers.length > 0;
        if (!items.length && !sizePicker) return null;
        const preset = sizePicker && selectedNativeImagePreset(variant, values);
        const sizeLabel = preset
            ? [preset.aspect_ratio, preset.resolution].join(" · ")
            : sizePointers
                  .map((key) => display(values[key], group))
                  .filter(Boolean)
                  .join(" · ");
        const summary = group === "更多参数" ? group : [sizePicker ? sizeLabel : "", ...items.map(([key]) => display(values[key], group))].filter(Boolean).join(" · ") || group;
        return menu(
            group,
            summary,
            icon,
            <>
                {sizePicker && <PlatformNativeImageSize key={sizeResetRevision} variant={variant} values={values} onChange={(next) => onChange("nativeParameters", JSON.stringify(next))} />}
                {items.map(([key, field]) => {
                    const label = creativeParameterLabel(key, field.title);
                    let choices = field.enum;
                    if (Object.hasOwn(field, "const")) choices = [field.const];
                    if (
                        !choices &&
                        (group === "时长" || group === "张数") &&
                        field.type === "integer" &&
                        typeof field.minimum === "number" &&
                        typeof field.maximum === "number" &&
                        Number.isInteger(field.minimum) &&
                        Number.isInteger(field.maximum) &&
                        field.maximum >= field.minimum &&
                        field.maximum - field.minimum <= 60
                    ) {
                        const min = field.minimum;
                        choices = Array.from({ length: field.maximum - min + 1 }, (_, i) => min + i).filter((n) => !field.multipleOf || n % Number(field.multipleOf) === 0);
                    }
                    return (
                        <section key={key}>
                            <strong>{label}</strong>
                            {choices?.length ? (
                                <div className="studio-video-choices">
                                    {choices.map((value) => (
                                        <button
                                            type="button"
                                            key={JSON.stringify(value)}
                                            aria-pressed={values[key] === value}
                                            onClick={() => {
                                                update(key, value);
                                                if (group === "时长" || group === "画质" || group === "张数") setOpen(null);
                                            }}
                                        >
                                            {display(value, group)}
                                        </button>
                                    ))}
                                </div>
                            ) : (
                                <NativeParameterInput pointer={key} field={field} value={values[key]} onChange={(value) => update(key, value)} />
                            )}
                        </section>
                    );
                })}
                {image && (
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
    const audio = fields.find((entry) => fieldGroup(entry, image) === "声音");
    const switchable = audio && !Object.hasOwn(audio[1], "const") && (!audio[1].enum || [true, false].every((value) => audio[1].enum?.includes(value)));
    const sound = audio && (values[audio[0]] ?? audio[1].default) === true;
    return (
        <>
            {modeMenu}
            {parameterMenu("尺寸与分辨率", <Monitor />)}
            {image && parameterMenu("画质", <Sparkles />)}
            {image && parameterMenu("张数", <Layers />)}
            {parameterMenu("时长", <Clock3 />)}
            {switchable && (
                <button type="button" role="switch" aria-checked={Boolean(sound)} aria-label="生成声音" className="creation-chat-control" onClick={() => update(audio[0], !sound)}>
                    {sound ? <Volume2 /> : <VolumeX />}
                    <span>{sound ? "声音开" : "声音关"}</span>
                </button>
            )}
            {parameterMenu("更多参数", <SlidersHorizontal />)}
        </>
    );
}
