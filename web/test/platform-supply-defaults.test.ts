import { expect, test } from "bun:test";
import type { Schema } from "@cfworker/json-schema";
import { supplyChoiceAllowed, supplySelection } from "@/lib/platform-media-supply";
import type { NativeMediaVariant } from "@/lib/platform-native-media";

function video(schemas: Schema[]): NativeMediaVariant {
    return {
        id: "variant",
        gateway_protocol: "modelark.video",
        media_generation_v1: true,
        workbench_parameters: ["/resolution", "/duration", "/ratio"],
        defaults: { "/resolution": "1080p", "/duration": 4 },
        specifications_schema: {
            required: ["/resolution", "/duration"],
            properties: {
                "/resolution": { enum: ["1080p", "480p", "720p"] },
                "/duration": { enum: [3, 4, 6, 8] },
                "/ratio": { enum: ["21:9", "16:9"] },
                "/audio": { type: "boolean" },
            },
        },
        native_request_variant: {
            protocol: "modelark.video",
            path: "/video",
            modes: ["async"],
            request_schema: {},
            native_reference_inputs: [],
            supply_alternatives: schemas.map((request_schema) => ({ request_schema, native_reference_inputs: [] })),
        },
    };
}

const hd: Schema = { properties: { resolution: { const: "1080p" } }, required: ["ratio"] };
const sd: Schema = { properties: { resolution: { const: "480p" } } };
const low: Schema = { properties: { resolution: { const: "720p" } } };

test("a chosen resolution derives only its missing mandatory duration from the published control order", () => {
    const variant = video([{ properties: { resolution: { const: "1080p" }, duration: { enum: [8, 6, 4] } } }, low]);
    variant.defaults = { "/resolution": "720p", "/duration": 3 };
    const explicit = { "/resolution": "1080p" };
    expect(supplySelection(variant, explicit, {})).toMatchObject({ issue: "", values: { "/resolution": "1080p", "/duration": 4 } });
    expect(explicit).toEqual({ "/resolution": "1080p" });
    const stale = { ...explicit, "/duration": 3 };
    expect(supplySelection(variant, stale, {})).toMatchObject({ values: stale, supply: undefined });
    expect(supplySelection(variant, { ...explicit, "/duration": 6 }, {}).values["/duration"]).toBe(6);
});

test("a fresh video uses the unique complete supply preserving its published resolution and duration in either order", () => {
    for (const schemas of [
        [low, hd, sd],
        [sd, hd, low],
    ]) {
        const variant = video(schemas);
        const fresh = supplySelection(variant, {}, {});
        expect(fresh).toMatchObject({ issue: "", values: { "/resolution": "1080p", "/duration": 4, "/ratio": "21:9" } });
        expect(fresh.supply?.request_schema).toBe(hd);
        expect(supplySelection(variant, { "/ratio": "16:9" }, {}).values["/ratio"]).toBe("16:9");
        expect(supplySelection(variant, { "/resolution": "480p" }, {}).values).toEqual({ "/resolution": "480p", "/duration": 4 });
    }
});

test("alternative order cannot choose between equally preferred complete supplies, or invent a ratio outside the active controls", () => {
    const otherHd = { ...hd, properties: { ...hd.properties, ratio: { const: "16:9" } } };
    for (const schemas of [
        [hd, otherHd],
        [otherHd, hd],
    ]) {
        const variant = video(schemas);
        expect(supplySelection(variant, {}, {}).issue).toContain("明确选择");
        expect(supplySelection(variant, { "/ratio": "16:9" }, {}).issue).toBe("");
    }
    const hidden = video([hd]);
    hidden.workbench_parameters = ["/resolution", "/duration"];
    expect(supplySelection(hidden, {}, {}).issue).toContain("请选择必填参数 /ratio");
    expect(supplySelection(video([sd, low]), {}, {}).issue).toContain("明确选择");
});

test("paid options cannot resolve an ambiguous default and optional settings stay absent", () => {
    const charged = { ...hd, properties: { ...hd.properties, audio: { const: true } }, required: ["audio"] };
    const variant = video([sd, charged]);
    variant.workbench_parameters!.push("/audio");
    expect(supplySelection(variant, {}, {}).issue).toContain("明确选择");
    expect(supplySelection(video([hd]), {}, {}).values).not.toHaveProperty("/audio");
    const multi = video([{ required: ["audio"], properties: { audio: { enum: [true, false] } } }]);
    multi.workbench_parameters!.push("/audio");
    expect(supplySelection(multi, {}, {}).issue).toContain("请选择必填参数 /audio");
});

test.each(["modelark", "newapi"])("%s supplies can require a parameter to be absent at a specific resolution", (protocol) => {
    const nested = protocol === "newapi";
    const ratioKey = nested ? "aspect_ratio" : "ratio";
    const pointer = nested ? "/metadata/aspect_ratio" : "/ratio";
    const resolution = nested ? "/metadata/resolution" : "/resolution";
    const tier = (size: string, omit: boolean): Schema => {
        const schema = { properties: { resolution: { const: size } }, ...(omit ? { allOf: [{ not: { required: [ratioKey] } }] } : { required: [ratioKey] }) };
        return nested ? { properties: { metadata: schema } } : schema;
    };
    const noRatio = tier("480p", true);
    const allowRatio = tier("720p", false);
    const variant = video([noRatio, tier("720p", true), allowRatio]);
    variant.workbench_parameters = [resolution, pointer];
    variant.specifications_schema = { properties: { [resolution]: { enum: ["480p", "720p"] }, [pointer]: { enum: ["21:9", "16:9"] } }, required: [resolution] };
    const low = { [resolution]: "480p" };
    const high = { [resolution]: "720p" };
    expect(supplyChoiceAllowed(variant, pointer, low, {})("21:9")).toBe(false);
    expect(supplyChoiceAllowed(variant, pointer, high, {})("21:9")).toBe(true);
    expect(supplySelection(variant, low, {}).supply?.request_schema).toBe(noRatio);
    expect(supplySelection(variant, { ...low, [pointer]: "21:9" }, {}).supply).toBeUndefined();
    expect(supplySelection(variant, { ...high, [pointer]: "21:9" }, {}).supply?.request_schema).toBe(allowRatio);
});

test("a mandatory parent can come from the native request while its mandatory ratio comes from the supply", () => {
    const pointer = "/metadata/aspect_ratio";
    const variant = video([{ properties: { metadata: { required: ["aspect_ratio"] } } }]);
    variant.workbench_parameters = [pointer];
    variant.specifications_schema = { properties: { [pointer]: { enum: ["16:9", "9:16"] } } };
    expect(supplySelection(variant, {}, {}).values).toEqual({});
    variant.native_request_variant.request_schema = { required: ["metadata"], properties: { metadata: { type: "object" } } };
    expect(supplySelection(variant, {}, {})).toMatchObject({ issue: "", values: { [pointer]: "16:9" } });
});
