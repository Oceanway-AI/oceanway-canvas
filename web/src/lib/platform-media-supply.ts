import { Validator, type Schema } from "@cfworker/json-schema";
import { creativeParameterLabel } from "./platform-media-label";
import { isNativeImageVariant, nativeImageSizeIssue } from "./platform-native-image-size";
import type { NativeImageOutputSize, NativeMediaVariant } from "./platform-native-media";

export type MaterialCounts = Record<string, number>;
export type SupplySlot = { role: string; pointer?: string; transport?: string; min_items?: number; max_items?: number };
export type NativeSupply = {
    request_schema?: Schema;
    native_reference_inputs: SupplySlot[];
    output_size?: NativeImageOutputSize;
    material_constraints?: { max_items?: number; audio_requires_visual?: boolean };
};

export function suppliesFor(variant: NativeMediaVariant): NativeSupply[] {
    const native = variant.native_request_variant;
    if (native.supply_alternatives === undefined) return [{ native_reference_inputs: native.native_reference_inputs ?? [], material_constraints: native.material_constraints, output_size: native.output_size }];
    if (!Array.isArray(native.supply_alternatives)) return [];
    return native.supply_alternatives.filter(
        (supply) => supply?.request_schema && typeof supply.request_schema === "object" && Array.isArray(supply.native_reference_inputs) && supply.native_reference_inputs.every((slot) => typeof slot?.role === "string"),
    );
}

function pointerPath(pointer: string) {
    return pointer
        .slice(1)
        .split("/")
        .map((key) => key.replaceAll("~1", "/").replaceAll("~0", "~"));
}

export function isImageOutputCountPointer(pointer: string) {
    return ["n", "candidatecount", "numimages", "numoutputs"].includes(pointerPath(pointer).at(-1)?.replaceAll("_", "").toLowerCase() ?? "");
}

function publishedCountIssue(variant: NativeMediaVariant, explicit: Record<string, unknown>) {
    if (!isNativeImageVariant(variant)) return "";
    for (const [pointer, value] of Object.entries(explicit)) {
        if (!isImageOutputCountPointer(pointer)) continue;
        const field = variant.specifications_schema.properties[pointer];
        if (!field || validField(field as Schema, value)) continue;
        const fixed = field.const ?? (field.enum?.length === 1 ? field.enum[0] : undefined) ?? (field.minimum === field.maximum ? field.maximum : undefined);
        if (typeof fixed === "number") return `当前型号的生成张数仅支持 ${fixed} 张`;
        if (typeof field.minimum === "number" && typeof field.maximum === "number") return `当前型号的生成张数支持 ${field.minimum} 至 ${field.maximum} 张`;
        return "当前型号不支持所选生成张数";
    }
    return "";
}

function forbidsProperty(schema: Schema | undefined, key: string): boolean {
    const excluded = schema?.not;
    if (excluded?.required?.length === 1 && excluded.required[0] === key && Object.keys(excluded).length === 1) return true;
    return schema?.allOf?.some((rule) => forbidsProperty(rule, key)) ?? false;
}

function schemaField(schema: Schema | undefined, pointer: string, explicit: Record<string, unknown>, paired?: Schema) {
    let required = true;
    let prefix = "";
    for (const key of pointerPath(pointer)) {
        const parentPresent = !prefix || Object.keys(explicit).some((path) => path.startsWith(prefix + "/"));
        required = (required || parentPresent) && Boolean(schema?.required?.includes(key) || paired?.required?.includes(key));
        const child: Schema | boolean | undefined = schema?.properties?.[key] ?? schema?.additionalProperties;
        if (child === false || forbidsProperty(schema, key)) return { required, forbidden: true, schema: undefined };
        schema = typeof child === "object" ? child : undefined;
        const other: Schema | boolean | undefined = paired?.properties?.[key] ?? paired?.additionalProperties;
        paired = typeof other === "object" ? other : undefined;
        prefix += "/" + key.replaceAll("~", "~0").replaceAll("/", "~1");
    }
    return { schema, required, forbidden: false };
}

function requiredParameter(variant: NativeMediaVariant, pointer: string, explicit: Record<string, unknown>, supply?: NativeSupply) {
    const branch = schemaField(supply?.request_schema, pointer, explicit, variant.native_request_variant.request_schema);
    const common = schemaField(variant.native_request_variant.request_schema, pointer, explicit);
    return {
        branch: branch.schema,
        common: common.schema,
        required: variant.specifications_schema.required?.includes(pointer) || common.required || branch.required || (pointer === "/size" && Boolean(supply?.output_size || variant.native_request_variant.output_size)),
    };
}

function sizeIssue(variant: NativeMediaVariant, values: Record<string, unknown>, limits?: NativeImageOutputSize) {
    try {
        return nativeImageSizeIssue({ ...variant, native_request_variant: { ...variant.native_request_variant, output_size: limits } }, values);
    } catch {
        return "模型尺寸规格无效，请刷新模型目录";
    }
}

function orderedVideoControl(variant: NativeMediaVariant, pointer: string) {
    const name = pointerPath(pointer).at(-1)?.replaceAll("_", "").toLowerCase();
    return variant.gateway_protocol.endsWith(".video") && variant.workbench_parameters?.includes(pointer) && ["duration", "seconds", "ratio", "aspectratio"].includes(name ?? "");
}

// The form owns only the published parameter pointers. Transport, resolved
// resources and prompt are validated on the fully assembled body by the host.
export function supplyParameters(variant: NativeMediaVariant, explicit: Record<string, unknown>, supply?: NativeSupply) {
    const values = { ...explicit };
    const native = variant.native_request_variant;
    const limits = supply?.output_size ?? native.output_size;
    for (const [pointer, field] of Object.entries(variant.specifications_schema.properties)) {
        if (Object.hasOwn(values, pointer)) continue;
        const { required, branch, common } = requiredParameter(variant, pointer, explicit, supply);
        if (!required) continue;
        const candidates = [branch?.const, common?.const, field.const, branch?.default, common?.default, variant.defaults?.[pointer], field.default];
        for (const source of [branch, common, field]) {
            if (source?.enum?.length === 1) candidates.push(source.enum[0]);
        }
        if (supply && orderedVideoControl(variant, pointer) && field.enum) candidates.push(...field.enum);
        if (pointer === "/size" && limits && !limits.allow_auto && limits.min_width > 0 && limits.min_width === limits.max_width && limits.min_height > 0 && limits.min_height === limits.max_height) {
            candidates.push(`${limits.min_width}x${limits.min_height}`);
        }
        const value = candidates.find((candidate) => {
            if (candidate === undefined) return false;
            if (!validField(branch, candidate) || !validField(common, candidate) || !validField(field as Schema, candidate)) return false;
            if (pointer !== "/size") return true;
            return !sizeIssue(variant, { ...values, [pointer]: candidate }, limits);
        });
        if (value !== undefined) values[pointer] = value;
    }
    return values;
}

export function slotRange(slots: SupplySlot[], role: string) {
    const matching = slots.filter((slot) => slot.role === role);
    return {
        min: matching.reduce((sum, slot) => sum + (slot.min_items ?? 0), 0),
        max: matching.reduce((sum, slot) => sum + (slot.max_items ?? Infinity), 0),
    };
}

const roleLabels: Record<string, string> = { reference_image: "参考图", first_frame: "首帧", last_frame: "尾帧", reference_video: "参考视频", reference_audio: "参考音频", mask: "蒙版" };

// A missing media field can rule out a schema choice. Prompt, transport and
// other non-material conditions remain the host's responsibility.
function missingReferenceGroup(schema: Schema | undefined, slots: SupplySlot[], counts: MaterialCounts): string[] | undefined {
    if (!schema) return;
    for (const pointer of new Set(slots.map((slot) => slot.pointer).filter((p): p is string => typeof p === "string" && p.startsWith("/")))) {
        const field = schemaField(schema, pointer, {});
        const bound = slots.filter((slot) => slot.pointer === pointer);
        const roles = [...new Set(bound.map((slot) => slot.role))];
        const countFor = (group: string[]) => group.reduce((sum, role) => sum + (counts[role] ?? 0), 0);
        if (bound.every((slot) => slot.transport === "content-array")) {
            // A content array already contains the prompt. Only a positive
            // contains predicate requiring a media role implies missing input.
            for (const rule of [field.schema, ...(field.schema?.allOf ?? [])]) {
                const item = rule?.contains;
                if (!item || typeof item !== "object" || !item.required?.includes("role")) continue;
                const roleField = item.properties?.role;
                if (!roleField || typeof roleField !== "object") continue;
                const matching = roles.filter((role) => validField(roleField, role));
                if (matching.length && countFor(matching) < (rule.minContains ?? 1)) return matching;
            }
        } else if (field.required && bound.every((slot) => ["url", "url-array", "image-reference", "image-reference-array"].includes(slot.transport ?? "")) && !countFor(roles)) {
            return roles;
        }
    }
    for (const rule of schema.allOf ?? []) {
        const missing = missingReferenceGroup(rule, slots, counts);
        if (missing) return missing;
    }
    if (schema.anyOf?.length) {
        const alternatives = schema.anyOf.map((rule) => missingReferenceGroup(rule, slots, counts));
        if (alternatives.every((roles) => roles !== undefined)) return [...new Set(alternatives.flat())];
    }
}

export function materialsIssue(variant: NativeMediaVariant, supply: NativeSupply, counts: MaterialCounts, allowMissing = false) {
    const roles = new Set([...Object.keys(counts), ...supply.native_reference_inputs.map((slot) => slot.role)]);
    for (const role of roles) {
        const count = counts[role] ?? 0;
        if (!Number.isSafeInteger(count) || count < 0) return "参考素材数量无效";
        const { min, max } = slotRange(supply.native_reference_inputs, role);
        const unit = role === "reference_image" ? "张" : "个";
        if (count > max) return `当前档位最多支持 ${max} ${unit}${roleLabels[role] || role}，已添加 ${count} ${unit}`;
        if (!allowMissing && count < min) return `当前档位至少需要 ${min} ${unit}${roleLabels[role] || role}`;
    }
    const policy = supply.material_constraints;
    if (policy?.max_items !== undefined && Object.values(counts).reduce((a, b) => a + b, 0) > policy.max_items) return `当前档位最多支持 ${policy.max_items} 个参考素材`;
    if (!allowMissing && policy?.audio_requires_visual && counts.reference_audio && !(counts.reference_image || counts.first_frame || counts.last_frame || counts.reference_video)) return "参考音频需要搭配图片或视频";
    if (!allowMissing) {
        const native = variant.native_request_variant;
        const missing = missingReferenceGroup(native.request_schema, native.native_reference_inputs ?? [], counts) ?? missingReferenceGroup(supply.request_schema, supply.native_reference_inputs, counts);
        if (missing) {
            const options = missing.filter((role) => slotRange(supply.native_reference_inputs, role).max > 0).map((role) => roleLabels[role] || role);
            return options.length ? `请补充当前规格要求的${options.join("或")}` : "当前规格缺少可用的必需参考素材";
        }
    }
    return "";
}

const validators = new WeakMap<object, Validator>();
function validField(schema: Schema | undefined, value: unknown) {
    if (!schema) return true;
    try {
        let validator = validators.get(schema);
        if (!validator) {
            validator = new Validator(structuredClone(schema), "2020-12");
            validators.set(schema, validator);
        }
        return validator.validate(value).valid;
    } catch {
        return false;
    }
}

function supplyAllowsValue(variant: NativeMediaVariant, supply: NativeSupply, pointer: string, value: unknown, explicit: Record<string, unknown>) {
    const field = variant.specifications_schema.properties[pointer];
    const branch = schemaField(supply.request_schema, pointer, explicit);
    const common = schemaField(variant.native_request_variant.request_schema, pointer, explicit);
    return Boolean(field && !branch.forbidden && !common.forbidden && validField(branch.schema, value) && validField(common.schema, value) && validField(field as Schema, value));
}

function resolutionParameter(pointer: string) {
    return ["size", "width", "height", "resolution", "videosize", "imagesize"].includes(pointerPath(pointer).at(-1)?.replaceAll("_", "").toLowerCase() ?? "");
}

export function suppliesAtResolution(variant: NativeMediaVariant, explicit: Record<string, unknown>, supplies = suppliesFor(variant)) {
    const selection = Object.entries(explicit).filter(([key]) => resolutionParameter(key));
    return supplies.filter((supply) =>
        selection.every(([key, value]) => {
            if (!supplyAllowsValue(variant, supply, key, value, explicit)) return false;
            return key !== "/size" || !sizeIssue(variant, { [key]: value }, supply.output_size ?? variant.native_request_variant.output_size);
        }),
    );
}

export function supplyChoiceAllowed(variant: NativeMediaVariant, pointer: string, explicit: Record<string, unknown>, counts?: MaterialCounts) {
    let supplies = suppliesFor(variant);
    if (!resolutionParameter(pointer)) {
        supplies = suppliesAtResolution(variant, explicit, supplies);
        if (counts) {
            const complete = supplies.filter((supply) => !materialsIssue(variant, supply, counts));
            // A missing required frame can be added; an excess attachment needs removing first.
            supplies = complete.length ? complete : supplies.filter((supply) => !materialsIssue(variant, supply, counts, true));
        }
        const other = Object.entries(explicit).filter(([key]) => key !== pointer);
        const compatible = supplies.filter((supply) => other.every(([key, value]) => supplyAllowsValue(variant, supply, key, value, explicit)));
        if (compatible.length) supplies = compatible;
    }
    return (value: unknown) => supplies.some((supply) => supplyAllowsValue(variant, supply, pointer, value, explicit));
}

export function supplyControlFields(variant: NativeMediaVariant, explicit: Record<string, unknown>, values: Record<string, unknown>, counts?: MaterialCounts) {
    const tier = { ...Object.fromEntries(Object.entries(values).filter(([pointer]) => resolutionParameter(pointer))), ...explicit };
    return Object.entries(variant.specifications_schema.properties).flatMap(([pointer, field]) => {
        if (variant.workbench_parameters && !variant.workbench_parameters.includes(pointer)) return [];
        const allowed = supplyChoiceAllowed(variant, pointer, tier, counts);
        const choices = Object.hasOwn(field, "const") ? [field.const] : (field.enum ?? (field.type === "boolean" ? [true, false] : undefined));
        const unavailable = choices !== undefined && !choices.some(allowed);
        return unavailable && !Object.hasOwn(explicit, pointer) ? [] : [{ pointer, field, allowed, unavailable }];
    });
}

export function supplyParameterIssue(variant: NativeMediaVariant, supply: NativeSupply, explicit: Record<string, unknown>, values = supplyParameters(variant, explicit, supply)) {
    const native = variant.native_request_variant;
    const sizeError = sizeIssue(variant, values, supply.output_size ?? native.output_size);
    if (sizeError) return sizeError;
    for (const pointer of Object.keys(variant.specifications_schema.properties)) {
        if (!Object.hasOwn(values, pointer) && requiredParameter(variant, pointer, explicit, supply).required) return `请选择必填参数 ${pointer}`;
    }
    for (const [pointer, value] of Object.entries(values)) {
        const field = variant.specifications_schema.properties[pointer];
        if (!field || (Object.hasOwn(explicit, pointer) && variant.workbench_parameters && !variant.workbench_parameters.includes(pointer))) return `当前组合未开放参数 ${pointer}`;
        if (!supplyAllowsValue(variant, supply, pointer, value, explicit)) {
            const label = field.title ? creativeParameterLabel(pointer, field.title) : pointer;
            return `当前组合不支持 ${label} 的所选值`;
        }
    }
    return "";
}

export function supplySelection(variant: NativeMediaVariant, explicit: Record<string, unknown>, counts?: MaterialCounts) {
    let materialIssue = "";
    let parameterIssue = "";
    const base = supplyParameters(variant, explicit);
    const all = suppliesFor(variant);
    const tier = suppliesAtResolution(variant, explicit, all);
    if (counts && tier.length && tier.every((supply) => materialsIssue(variant, supply, counts, true))) {
        return { supply: undefined, values: explicit, issue: materialsIssue(variant, tier[0], counts, true) };
    }
    const candidates = new Map<string, { supply: NativeSupply; values: Record<string, unknown>; issue: string }>();
    for (const supply of tier.length ? tier : all) {
        const values = supplyParameters(variant, explicit, supply);
        const issue = supplyParameterIssue(variant, supply, explicit, values);
        if (issue) {
            parameterIssue ||= issue;
            continue;
        }
        const conflict = counts && materialsIssue(variant, supply, counts);
        if (conflict) {
            materialIssue ||= conflict;
            continue;
        }
        const result = { supply, values, issue: "" };
        const signature = JSON.stringify(values);
        if (signature === JSON.stringify(base)) return result;
        if (!candidates.has(signature)) candidates.set(signature, result);
    }
    if (candidates.size === 1) return [...candidates.values()][0];
    if (candidates.size > 1) {
        // A tier can require a public duration or ratio that is optional at the model level.
        const preferred = [...candidates.values()].filter(({ values }) => Object.entries(base).every(([key, value]) => Object.is(values[key], value)) && Object.keys(values).every((key) => Object.hasOwn(base, key) || orderedVideoControl(variant, key)));
        if (preferred.length === 1) return preferred[0];
        return { supply: undefined, values: explicit, issue: "多个供给规格需要不同的必填值，请明确选择生成参数" };
    }
    return { supply: undefined, values: explicit, issue: materialIssue || publishedCountIssue(variant, explicit) || parameterIssue || "当前型号没有可用的供给组合" };
}

export function mediaRoleCounts(materials: ReadonlyArray<{ kind: string; role?: string }>): MaterialCounts {
    const counts: MaterialCounts = {};
    for (const material of materials) {
        const kind = material.kind === "character" ? "image" : material.kind;
        if (!["image", "video", "audio", "mask"].includes(kind)) continue;
        const role = kind === "mask" ? "mask" : material.role && material.role !== "reference" ? material.role : `reference_${kind}`;
        counts[role] = (counts[role] ?? 0) + 1;
    }
    return counts;
}
