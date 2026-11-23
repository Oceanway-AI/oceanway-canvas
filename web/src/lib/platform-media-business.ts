import type { NativeMediaVariant, NativeSchemaField } from "./platform-native-media";
import { slotRange, suppliesAtResolution, suppliesFor, supplyChoiceAllowed, supplyControlFields, supplySelection, type MaterialCounts } from "./platform-media-supply";

type Values = Record<string, unknown>;

// These published fields have the same unit and meaning across native APIs.
// All other controls require their exact JSON pointer.
const aliases = [
    ["/resolution", "/metadata/resolution", "/generationConfig/imageConfig/imageSize"],
    ["/ratio", "/aspect_ratio", "/metadata/aspect_ratio", "/generationConfig/imageConfig/aspectRatio"],
    ["/duration", "/seconds"],
    ["/generate_audio", "/metadata/generate_audio"],
];

function businessKey(pointer: string) {
    return aliases.find((group) => group.includes(pointer))?.[0] ?? pointer;
}

function controlPointers(variant: NativeMediaVariant) {
    return variant.workbench_parameters ?? Object.keys(variant.specifications_schema.properties);
}

function targetPointer(variant: NativeMediaVariant, pointer: string) {
    const fields = controlPointers(variant);
    if (fields.includes(pointer)) return pointer;
    const targets = fields.filter((key) => businessKey(key) === businessKey(pointer));
    if (targets.length === 1) return targets[0];
}

function nativeValue(pointer: string, value: unknown, field: NativeSchemaField) {
    if (businessKey(pointer) !== "/duration") return value;
    if (typeof value === "string" && /^-?(0|[1-9]\d*)$/.test(value) && Number.isSafeInteger(Number(value))) {
        if (field.type === "integer" || field.type === "number" || field.enum?.some((v) => typeof v === "number") || typeof field.const === "number") return Number(value);
    } else if (typeof value === "number" && Number.isSafeInteger(value)) {
        if (field.type === "string" || field.enum?.some((v) => typeof v === "string") || typeof field.const === "string") return String(value);
    }
    return value;
}

export function publishedParameters(variant: NativeMediaVariant, explicit: Values) {
    const parameters: Values = {};
    for (const [pointer, value] of Object.entries(explicit)) {
        const target = targetPointer(variant, pointer);
        if (target && target !== pointer && !Object.hasOwn(explicit, target) && !Object.hasOwn(parameters, target)) {
            parameters[target] = nativeValue(target, value, variant.specifications_schema.properties[target]);
        } else {
            parameters[pointer] = value;
        }
    }
    return parameters;
}

export function selectBusinessParameter(explicit: Values, pointer: string, value: unknown) {
    const same = businessKey(pointer);
    const next = Object.fromEntries(Object.entries(explicit).filter(([key]) => businessKey(key) !== same));
    if (value !== undefined) next[pointer] = value;
    return next;
}

export function materialGroup(variant: NativeMediaVariant) {
    const roles = variant.native_request_variant.native_reference_inputs?.filter((slot) => slot.max_items !== 0).map((slot) => slot.role) ?? [];
    if (!roles.length) return "text";
    if (roles.some((role) => role === "first_frame" || role === "last_frame" || role === "mask" || role === "markup")) return [...new Set(roles)].sort().join(",");
    return "reference";
}

export function businessRoutes(variants: NativeMediaVariant[], base: NativeMediaVariant, counts: MaterialCounts = {}) {
    const ordered = [base, ...variants.filter((variant) => variant.id !== base.id)];
    if (base.gateway_protocol.endsWith(".video")) return ordered.filter((variant) => materialGroup(variant) === materialGroup(base));
    const matches = ordered.filter((variant) =>
        suppliesFor(variant).some((supply) => {
            return ["reference_image", "mask"].every((role) => {
                const count = counts[role] ?? 0;
                const { min, max } = slotRange(supply.native_reference_inputs, role);
                return min <= count && count <= max;
            });
        }),
    );
    return matches.length ? matches : [base];
}

export function chooseBusinessRoute(variants: NativeMediaVariant[], explicit: Values, counts: MaterialCounts) {
    const matches = variants.map((variant) => {
        const parameters = publishedParameters(variant, explicit);
        return { variant, parameters, issue: supplySelection(variant, parameters, counts).issue, inTier: suppliesAtResolution(variant, parameters).length > 0, exact: Object.keys(explicit).every((key) => controlPointers(variant).includes(key)) };
    });
    // Prefer the saved spelling on reissue. For an invalid draft retain its
    // selected resolution, so the user can correct the duration or attachments.
    matches.sort((a, b) => Number(!b.issue) - Number(!a.issue) || Number(b.inTier) - Number(a.inTier) || Number(b.exact) - Number(a.exact));
    return matches[0];
}

export function finiteBusinessChoices(field: NativeSchemaField, includeRange = true): unknown[] | undefined {
    if (Object.hasOwn(field, "const")) return [field.const];
    if (field.enum) return field.enum;
    if (field.type === "boolean") return [true, false];
    if (!includeRange || field.type !== "integer" || typeof field.minimum !== "number" || typeof field.maximum !== "number") return;
    const min = Math.ceil(field.minimum),
        max = Math.floor(field.maximum);
    if (!Number.isSafeInteger(min) || !Number.isSafeInteger(max) || max < min || max - min > 60) return;
    return Array.from({ length: max - min + 1 }, (_, i) => min + i).filter((n) => !field.multipleOf || n % Number(field.multipleOf) === 0);
}

export function businessControlFields(variants: NativeMediaVariant[], base: NativeMediaVariant, explicit: Values, values: Values, counts?: MaterialCounts) {
    const routes = businessRoutes(variants, base, counts);
    if (routes.length === 1 && routes[0] === base) return supplyControlFields(base, explicit, values, counts);
    const groups = new Map<string, { variant: NativeMediaVariant; pointer: string; field: NativeSchemaField }[]>();
    for (const variant of routes) {
        for (const pointer of controlPointers(variant)) {
            const field = variant.specifications_schema.properties[pointer];
            if (!field) continue;
            const key = businessKey(pointer);
            const list = groups.get(key) ?? [];
            list.push({ variant, pointer, field });
            groups.set(key, list);
        }
    }
    const tier = { ...Object.fromEntries(Object.entries(values).filter(([key]) => businessKey(key) === "/resolution" || key === "/size")), ...explicit };
    return [...groups.values()].flatMap((entries) => {
        const first = entries[0];
        const options = entries.map(({ field }) => finiteBusinessChoices(field));
        const choices = options.every((v) => v !== undefined) ? [...new Set(options.flat().map((v) => nativeValue(first.pointer, v, first.field)))] : undefined;
        const matchers = entries.map(({ variant, pointer, field }) => {
            const parameters = publishedParameters(variant, tier);
            return { pointer, field, strict: supplyChoiceAllowed(variant, pointer, parameters, counts, true), repair: supplyChoiceAllowed(variant, pointer, parameters, counts) };
        });
        const matches = (mode: "strict" | "repair") => (value: unknown) => matchers.some((row) => row[mode](nativeValue(row.pointer, value, row.field)));
        const strict = matches("strict");
        const allowed = choices && !choices.some(strict) ? matches("repair") : strict;
        const unavailable = choices !== undefined && !choices.some(allowed);
        if (unavailable && !Object.hasOwn(explicit, first.pointer)) return [];
        const field = { ...first.field };
        if (choices && (choices.length > 1 || !Object.hasOwn(field, "const"))) {
            delete field.const;
            field.enum = choices;
        } else if (!choices) {
            const open = entries.find((entry) => !finiteBusinessChoices(entry.field));
            if (open) {
                Object.assign(field, open.field);
                delete field.const;
                delete field.enum;
            }
        }
        return [{ pointer: first.pointer, field, allowed, unavailable }];
    });
}
