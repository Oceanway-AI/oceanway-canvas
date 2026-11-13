import { Input } from "antd";
import { useState } from "react";
import { ImageParameterInput } from "./image-parameter-fields";
import { Select } from "@/components/ui/base/select";
import type { ImageParameter } from "@/lib/image-parameters";
import { creativeParameterLabel, nativeMediaProfile, nativeMediaVariant, nativeMediaVariantOptions, nativeCreationModeOptions, nativeParameterValues, type NativeSchemaField } from "@/lib/platform-native-media";
import type { AiConfig } from "@/stores/use-config-store";
import { nativeImageSizePointers } from "@/lib/platform-native-image-size";
import { PlatformNativeImageSize } from "./platform-native-image-size";

export type NativeMediaSettingKey = "nativeParameters" | "nativeVariantId";

export function PlatformNativeMediaSettings({ config, onChange }: { config: AiConfig; onChange: (key: NativeMediaSettingKey, value: string) => void }) {
    const [sizeResetRevision, setSizeResetRevision] = useState(0);
    const profile = nativeMediaProfile(config);
    const variant = nativeMediaVariant(config);
    const controls = variant?.workbench_parameters;
    const automaticImageMode = controls && profile?.variants.every((v) => ["openai.image.generate", "openai.image.edit"].includes(v.gateway_protocol));
    let values: Record<string, unknown> = {};
    let error = "";
    let sizePointers: string[] = [];
    try {
        if (variant) {
            values = nativeParameterValues(variant, config.nativeParameters);
            const pointers = nativeImageSizePointers(variant);
            if (!controls || pointers.every((key) => controls.includes(key))) sizePointers = pointers;
        }
    } catch {
        error = "原生参数格式无效，请恢复模型默认值";
    }
    const change = (pointer: string, value: unknown) => {
        const next = { ...values };
        if (value === undefined) delete next[pointer];
        else next[pointer] = value;
        onChange("nativeParameters", JSON.stringify(next));
    };
    return (
        <div className="min-w-0 space-y-3" data-canvas-no-zoom data-canvas-wheel-scroll onMouseDown={(e) => e.stopPropagation()}>
            {profile && profile.variants.length > 1 && !automaticImageMode ? (
                <Select
                    className="w-full"
                    ariaLabel={controls ? "生成模式" : "请求协议"}
                    value={variant?.id}
                    placeholder={controls ? "选择生成方式" : "选择请求协议"}
                    options={controls ? nativeCreationModeOptions(profile.variants, variant) : nativeMediaVariantOptions(profile.variants)}
                    virtual={false}
                    optionRender={(option) => <span className="block whitespace-normal break-words">{option.label}</span>}
                    onChange={(id) => {
                        onChange("nativeParameters", "{}");
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
                    {sizePointers.length ? <PlatformNativeImageSize key={`${config.model}:${variant.id}:${sizeResetRevision}`} variant={variant} values={values} onChange={(next) => onChange("nativeParameters", JSON.stringify(next))} /> : null}
                    {Object.entries(variant.specifications_schema.properties)
                        .filter(([pointer]) => !sizePointers.includes(pointer) && (!controls || controls.includes(pointer)))
                        .sort((a, b) => Number(a[1]["x-order"] || 0) - Number(b[1]["x-order"] || 0))
                        .map(([pointer, field]) => (
                            <div key={`${variant.id}:${pointer}`} className="min-w-0 space-y-1">
                                <div className="break-all text-xs">
                                    {controls ? creativeParameterLabel(pointer, field.title) : pointer}
                                    {variant.specifications_schema.required?.includes(pointer) ? " *" : ""}
                                </div>
                                <NativeParameterInput pointer={pointer} field={field} value={values[pointer]} onChange={(value) => change(pointer, value)} />
                                {field.description ? <p className="text-xs opacity-60">{field.description}</p> : null}
                            </div>
                        ))}
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

export function NativeParameterInput({ pointer, field, value, onChange }: { pointer: string; field: NativeSchemaField; value: unknown; onChange: (v: unknown) => void }) {
    if (Object.hasOwn(field, "const")) return <Input aria-label={pointer} value={JSON.stringify(field.const)} readOnly />;
    if (field.enum?.length)
        return (
            <Select
                ariaLabel={pointer}
                className="w-full"
                allowClear
                value={value === undefined ? undefined : JSON.stringify(value)}
                options={field.enum.map((v) => ({ value: JSON.stringify(v), label: typeof v === "string" ? v : JSON.stringify(v) }))}
                onChange={(v) => onChange(v === undefined ? undefined : JSON.parse(String(v)))}
            />
        );
    if (typeof field.type === "string" && ["string", "number", "integer", "boolean"].includes(field.type)) {
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
