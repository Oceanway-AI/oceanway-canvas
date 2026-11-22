import { Input } from "antd";
import { useState } from "react";
import { ImageParameterInput } from "./image-parameter-fields";
import { Select } from "@/components/ui/base/select";
import type { ImageParameter } from "@/lib/image-parameters";
import { creativeParameterLabel, nativeMediaProfile, nativeMediaVariant, nativeMediaVariantOptions, nativeCreationModeOptions, nativeParameterObject, type NativeSchemaField } from "@/lib/platform-native-media";
import type { AiConfig } from "@/stores/use-config-store";
import { nativeImageSizePointers } from "@/lib/platform-native-image-size";
import { supplyChoiceAllowed, supplySelection, type MaterialCounts } from "@/lib/platform-media-supply";
import { imageSizeDisplayValues, PlatformNativeImageSize } from "./platform-native-image-size";

export type NativeMediaSettingKey = "nativeParameters" | "nativeVariantId";

export function PlatformNativeMediaSettings({ config, references, onChange }: { config: AiConfig; references?: MaterialCounts; onChange: (key: NativeMediaSettingKey, value: string) => void }) {
    const [sizeResetRevision, setSizeResetRevision] = useState(0);
    const profile = nativeMediaProfile(config);
    const variant = nativeMediaVariant(config);
    const controls = variant?.workbench_parameters;
    let explicit: Record<string, unknown> = {};
    let values: Record<string, unknown> = {};
    let error = "";
    let selectionIssue = "";
    let sizePointers: string[] = [];
    try {
        if (variant) {
            explicit = nativeParameterObject(config.nativeParameters);
            const selection = supplySelection(variant, explicit, references);
            values = selection.values;
            const pointers = nativeImageSizePointers(variant);
            if (!controls || pointers.every((key) => controls.includes(key))) sizePointers = pointers;
            if (!sizePointers.length) selectionIssue = selection.issue;
        }
    } catch {
        error = "原生参数格式无效，请恢复模型默认值";
    }
    const change = (pointer: string, value: unknown) => {
        const next = { ...explicit };
        if (value === undefined) delete next[pointer];
        else next[pointer] = value;
        onChange("nativeParameters", JSON.stringify(next));
    };
    return (
        <div className="min-w-0 space-y-3" data-canvas-no-zoom data-canvas-wheel-scroll onMouseDown={(e) => e.stopPropagation()}>
            {profile && profile.variants.length > 1 && profile.variants.some((v) => v.gateway_protocol.endsWith(".video")) ? (
                <Select
                    className="w-full"
                    ariaLabel={controls ? "生成模式" : "请求协议"}
                    value={variant?.id}
                    placeholder={controls ? "选择生成方式" : "选择请求协议"}
                    options={controls ? nativeCreationModeOptions(profile.variants, variant) : nativeMediaVariantOptions(profile.variants)}
                    virtual={false}
                    optionRender={(option) => <span className="block whitespace-normal break-words">{option.label}</span>}
                    onChange={(id) => {
                        onChange("nativeVariantId", String(id));
                    }}
                />
            ) : null}
            {!variant ? (
                <p role="alert">当前参数版本已失效，请重新选择模型。</p>
            ) : error ? (
                <p role="alert">{error}</p>
            ) : (
                <>
                    {sizePointers.length ? (
                        <PlatformNativeImageSize
                            key={`${config.model}:${variant.id}:${sizeResetRevision}`}
                            variant={variant}
                            explicit={explicit}
                            values={imageSizeDisplayValues(variant, values, references)}
                            references={references}
                            onChange={(next) => onChange("nativeParameters", JSON.stringify(next))}
                        />
                    ) : null}
                    {Object.entries(variant.specifications_schema.properties)
                        .filter(([pointer]) => !sizePointers.includes(pointer) && (!controls || controls.includes(pointer)))
                        .sort((a, b) => Number(a[1]["x-order"] || 0) - Number(b[1]["x-order"] || 0))
                        .map(([pointer, field]) => (
                            <div key={`${variant.id}:${pointer}`} className="min-w-0 space-y-1">
                                <div className="break-all text-xs">
                                    {controls ? creativeParameterLabel(pointer, field.title) : pointer}
                                    {variant.specifications_schema.required?.includes(pointer) ? " *" : ""}
                                </div>
                                <NativeParameterInput
                                    pointer={pointer}
                                    field={field}
                                    value={values[pointer]}
                                    required={variant.specifications_schema.required?.includes(pointer)}
                                    choiceAllowed={supplyChoiceAllowed(variant, pointer, explicit, references)}
                                    onChange={(value) => change(pointer, value)}
                                />
                                {field.description ? <p className="text-xs opacity-60">{field.description}</p> : null}
                            </div>
                        ))}
                    {selectionIssue && (
                        <p role="alert" className="text-xs">
                            {selectionIssue}
                        </p>
                    )}
                </>
            )}
            {!controls &&
                variant?.native_request_variant.native_reference_inputs?.map((ref) => (
                    <p key={ref.id} className="break-words text-xs opacity-70">
                        {ref.role} · {ref.min_items ?? 0}–{ref.max_items ?? "未限制"} · {ref.mime_types?.join(", ") || "按发布规格"}
                    </p>
                ))}
            <button
                type="button"
                className="text-xs underline"
                onClick={() => {
                    setSizeResetRevision((revision) => revision + 1);
                    onChange("nativeParameters", "{}");
                }}
            >
                恢复模型默认值
            </button>
        </div>
    );
}

function parameterText(value: unknown) {
    if (typeof value === "boolean") return value ? "开" : "关";
    return typeof value === "string" ? value : JSON.stringify(value);
}

export function NativeParameterInput({
    pointer,
    field,
    value,
    required = false,
    choiceAllowed,
    onChange,
}: {
    pointer: string;
    field: NativeSchemaField;
    value: unknown;
    required?: boolean;
    choiceAllowed?: (v: unknown) => boolean;
    onChange: (v: unknown) => void;
}) {
    if (Object.hasOwn(field, "const"))
        return (
            <>
                <Input aria-label={pointer} value={parameterText(field.const)} readOnly />
                {value !== undefined && JSON.stringify(value) !== JSON.stringify(field.const) && (
                    <button type="button" className="text-xs underline" onClick={() => onChange(undefined)}>
                        使用固定值
                    </button>
                )}
            </>
        );
    if (field.type === "boolean" || (field.enum?.length && field.enum.every((v) => typeof v === "boolean"))) {
        const allowed = field.enum ?? [true, false];
        return (
            <Select
                ariaLabel={pointer}
                className="w-full"
                value={value === undefined ? (required ? undefined : "default") : JSON.stringify(value)}
                placeholder="请选择"
                options={[...(required ? [] : [{ value: "default", label: "模型默认" }]), ...allowed.map((v) => ({ value: JSON.stringify(v), label: parameterText(v), disabled: choiceAllowed?.(v) === false }))]}
                onChange={(v) => onChange(v === "default" ? undefined : JSON.parse(String(v)))}
            />
        );
    }
    if (field.enum?.length)
        return (
            <Select
                ariaLabel={pointer}
                className="w-full"
                allowClear={!required}
                placeholder={required ? "请选择" : "模型默认"}
                value={value === undefined ? undefined : JSON.stringify(value)}
                options={field.enum.map((v) => ({ value: JSON.stringify(v), label: typeof v === "string" ? v : JSON.stringify(v), disabled: choiceAllowed?.(v) === false }))}
                onChange={(v) => onChange(v === undefined ? undefined : JSON.parse(String(v)))}
            />
        );
    if (typeof field.type === "string" && ["string", "number", "integer"].includes(field.type)) {
        const parameter: ImageParameter = {
            name: pointer,
            label: pointer,
            type: field.type as ImageParameter["type"],
            min: typeof field.minimum === "number" ? field.minimum : undefined,
            max: typeof field.maximum === "number" ? field.maximum : undefined,
            step: typeof field.multipleOf === "number" ? field.multipleOf : undefined,
            maxLength: typeof field.maxLength === "number" ? field.maxLength : undefined,
        };
        return <ImageParameterInput field={parameter} value={value as string | number | boolean | undefined} onChange={onChange} />;
    }
    // A malformed object/array is kept as a string, so full schema admission
    // rejects it instead of submitting the previous valid value invisibly.
    return (
        <Input.TextArea
            aria-label={pointer}
            autoSize={{ minRows: 2, maxRows: 6 }}
            value={value === undefined ? "" : typeof value === "string" ? value : JSON.stringify(value, null, 2)}
            onChange={(e) => {
                const raw = e.target.value;
                if (!raw) {
                    onChange(undefined);
                    return;
                }
                try {
                    onChange(JSON.parse(raw));
                } catch {
                    onChange(raw);
                }
            }}
        />
    );
}
