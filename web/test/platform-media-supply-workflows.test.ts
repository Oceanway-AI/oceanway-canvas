import { expect, test } from "bun:test";
import { catalogVariants } from "./fixtures/platform-partial-catalog";
import publishedGpt from "../../backend/internal/platformintegration/testdata/image-output-size/gpt-default.model.json";
import { imageExecutionChoices, nativeMediaReferenceLimits, type NativeMediaVariant } from "@/lib/platform-native-media";
import { supplyChoiceAllowed, supplySelection } from "@/lib/platform-media-supply";
import { modelCompatibilityError } from "@/lib/model-selection";
import { systemChannelModelChannels } from "@/lib/user-session";
import { prepareBackendGenerationTask } from "@/services/api/generation-task";
import type { PublicChannelModel } from "@/services/api/logical-models";
import { studioMediaPriceRequest } from "@/services/studio-media-price";
import { defaultConfig } from "@/stores/use-config-store";
import type { ReferenceImage } from "@/types/image";

function slot(role: string, max_items: number, min_items = 0) {
    return { id: role, pointer: "/images", transport: "url-array", role, min_items, max_items };
}

function configFor(mode: "image" | "video", variants: NativeMediaVariant[], parameters: Record<string, unknown>) {
    const model = {
        id: "fixture",
        modelKey: "fixture",
        displayName: "Fixture",
        capability: mode,
        available: true,
        platform: { model_type: mode, price: { version: "price-1" }, media_generation_profile: { gateway_protocol: variants[0].gateway_protocol, default_variant: variants[0].id, variants } },
    } as unknown as PublicChannelModel;
    const modelName = "oceanway-platform::fixture";
    const channels = systemChannelModelChannels([{ id: "oceanway-platform", name: "Platform", displayName: "Platform", models: [model] }]);
    return { ...defaultConfig, channels, model: modelName, imageModel: modelName, videoModel: modelName, nativeVariantId: variants[0].id, nativeParameters: JSON.stringify(parameters) };
}

function images(count: number): ReferenceImage[] {
    return Array.from({ length: count }, (_, i) => ({ id: String(i), name: String(i), type: "image/png", dataUrl: "", storageKey: `resource:pin-${i}`, role: "reference" }));
}

function requests(mode: "image" | "video", variants: NativeMediaVariant[], explicit: Record<string, unknown>, pins: ReferenceImage[]) {
    const config = configFor(mode, variants, explicit);
    const quote = studioMediaPriceRequest(
        config,
        mode,
        "A scene",
        pins.map((pin) => ({ kind: "image", storageKey: pin.storageKey, role: pin.role })),
        "owner",
    );
    const submit = () => prepareBackendGenerationTask({ mode, config, prompt: "A scene", referenceImages: pins });
    return { config, quote, submit };
}

const seconds = ["4", "5", "6", "8", "10"];
const imageSlot = slot("reference_image", 3);
const video: NativeMediaVariant = {
    id: "video",
    media_generation_v1: true,
    gateway_protocol: "newapi.video",
    defaults: { "/metadata/resolution": "720p", "/seconds": "5", "/metadata/generate_audio": true },
    specifications_schema: { properties: { "/metadata/resolution": { enum: ["720p", "1080p"] }, "/seconds": { title: "seconds", enum: seconds }, "/metadata/generate_audio": { type: "boolean" } } },
    native_request_variant: {
        protocol: "newapi.video",
        path: "/v1/videos",
        modes: ["async"],
        request_schema: {},
        native_reference_inputs: [imageSlot],
        supply_alternatives: [
            { request_schema: { properties: { metadata: { properties: { resolution: { const: "720p" } } }, seconds: { enum: seconds } } }, native_reference_inputs: [imageSlot] },
            { request_schema: { properties: { metadata: { properties: { resolution: { const: "1080p" } } }, seconds: { enum: ["4", "6", "8", "10"] } } }, native_reference_inputs: [slot("reference_image", 1)] },
        ],
    },
};

test.each([["720p", "5", 3, 3], ...["4", "6", "8", "10"].map((duration) => ["1080p", duration, 1, 1] as const)] as const)("video preview and admission preserve %s, %s seconds and %d ordinary images", async (resolution, duration, count, cap) => {
    const explicit = { "/metadata/resolution": resolution, "/seconds": duration };
    const pins = images(count);
    const { config, quote, submit } = requests("video", [video], explicit, pins);
    expect(nativeMediaReferenceLimits(config)).toMatchObject({ maxImages: cap, maxFirstFrames: 0, maxLastFrames: 0 });
    const task = await submit();
    expect(quote?.input?.config).toEqual(task.input?.config);
    expect(task.input?.config).toEqual({ channelId: "oceanway-platform", model: "fixture", nativeVariantId: video.id, nativeParameters: explicit });
    expect(quote?.input?.referenceImages).toEqual(pins.map(({ storageKey, role }) => ({ storageKey, role })));
    expect(task.input?.referenceImages).toMatchObject(quote!.input!.referenceImages);
});

test.each([
    ["1080p", "4", 3, "最多支持 1 张参考图"],
    ["720p", "5", 4, "最多支持 3 张参考图"],
    ["1080p", "5", 1, /所选值|时长/],
] as const)("video refuses the cross-supply combination %s, %s seconds and %d images without altering the draft", async (resolution, duration, count, issue) => {
    const explicit = { "/metadata/resolution": resolution, "/seconds": duration };
    const pins = images(count),
        originalPins = structuredClone(pins);
    const { config, quote, submit } = requests("video", [video], explicit, pins);
    expect(quote).toBeNull();
    await expect(submit()).rejects.toThrow(issue);
    expect(JSON.parse(config.nativeParameters)).toEqual(explicit);
    expect(pins).toEqual(originalPins);
});

test("a stale duration keeps the chosen resolution's reference limit and names the time to repair", async () => {
    const explicit = { "/metadata/resolution": "1080p", "/seconds": "5" };
    for (const ordered of [video.native_request_variant.supply_alternatives!, [...video.native_request_variant.supply_alternatives!].reverse()]) {
        const variant = { ...video, native_request_variant: { ...video.native_request_variant, supply_alternatives: ordered } };
        const { config, quote, submit } = requests("video", [variant], explicit, images(1));
        expect(nativeMediaReferenceLimits(config)?.maxImages).toBe(1);
        expect(quote).toBeNull();
        await expect(submit()).rejects.toThrow("时长");
        expect(JSON.parse(config.nativeParameters)).toEqual(explicit);
        const counts = { reference_image: 3 };
        expect(supplySelection(variant, explicit, counts).issue).toContain("最多支持 1 张参考图");
        expect(seconds.filter(supplyChoiceAllowed(variant, "/seconds", explicit, counts))).toEqual([]);
        expect(seconds.filter(supplyChoiceAllowed(variant, "/seconds", explicit, { reference_image: 1 }))).toEqual(["4", "6", "8", "10"]);
    }
});

test("a missing required frame can still be added to a draft with published durations", () => {
    const frame = { ...video, native_request_variant: { ...video.native_request_variant, supply_alternatives: [{ request_schema: {}, native_reference_inputs: [slot("first_frame", 1, 1)] }] } };
    expect(supplyChoiceAllowed(frame, "/seconds", {}, {})("5")).toBe(true);
    expect(supplySelection(frame, {}, {}).issue).toContain("至少需要");
});

test("ordinary video references cannot silently occupy available first and last frame slots", async () => {
    const slots = ["reference_image", "first_frame", "last_frame"].map((role) => slot(role, 1));
    const mixed = { ...video, native_request_variant: { ...video.native_request_variant, native_reference_inputs: slots, supply_alternatives: undefined } };
    const pins = images(3);
    pins[0].role = "last_frame";
    const invalid = requests("video", [mixed], {}, pins);
    expect(invalid.quote).toBeNull();
    await expect(invalid.submit()).rejects.toThrow("最多支持 1 张参考图");
    pins[2].role = "first_frame";
    const valid = requests("video", [mixed], {}, pins);
    const task = await valid.submit();
    expect(valid.quote?.input?.config).toEqual(task.input?.config);
    expect(valid.quote?.input?.referenceImages).toEqual([
        { storageKey: pins[2].storageKey, role: "first_frame" },
        { storageKey: pins[0].storageKey, role: "last_frame" },
        { storageKey: pins[1].storageKey, role: "reference" },
    ]);
    expect(task.input?.referenceImages).toMatchObject(valid.quote!.input!.referenceImages);
});

const gpt = publishedGpt.media_generation_profile.variants[0] as unknown as NativeMediaVariant;
const generate = { ...gpt, id: "generate", media_generation_v1: true };
test("equivalent image endpoints can be automatic, different paths require an explicit choice before quote or submit", async () => {
    const same = { ...generate, id: "same" };
    const alternative = { ...generate, id: "different", native_request_variant: { ...generate.native_request_variant, path: "/alternate/images" } };
    const params = { "/size": "1024x1024" };
    const automatic = requests("image", [generate, same], params, []);
    expect(imageExecutionChoices(automatic.config, {})).toEqual([]);
    expect((await automatic.submit()).platform?.variantId).toBe("generate");
    const ambiguous = requests("image", [generate, alternative], params, []);
    expect(imageExecutionChoices(ambiguous.config, {}).map((v) => v.id)).toEqual(["generate", "different"]);
    expect(modelCompatibilityError(ambiguous.config, ambiguous.config.model, { capability: "image", input: { imageCount: 0, characterCount: 0, videoCount: 0, audioCount: 0, textCount: 0 } })).toBe("");
    expect(ambiguous.quote).toBeNull();
    await expect(ambiguous.submit()).rejects.toThrow("请选择生成规格");
    const config = { ...ambiguous.config, nativeImageChoiceId: alternative.id };
    const task = await prepareBackendGenerationTask({ mode: "image", config, prompt: "A scene", referenceImages: [] });
    expect(task.platform?.variantId).toBe(alternative.id);
    expect(studioMediaPriceRequest(config, "image", "A scene", [], "owner")?.platform?.variantId).toBe(alternative.id);
});
const eightMP = { ...gpt.native_request_variant.output_size!, max_pixels: 8294400, allow_auto: true };
const fourMP = { ...eightMP, max_pixels: 4194304, allow_auto: false };
function edit(highLimit = 16): NativeMediaVariant {
    const slots = [slot("reference_image", 16, 1)];
    return {
        ...generate,
        id: "edit",
        gateway_protocol: "openai.image.edit",
        native_request_variant: {
            ...gpt.native_request_variant,
            protocol: "openai.image.edit",
            path: "/v1/images/edits",
            native_reference_inputs: slots,
            output_size: eightMP,
            supply_alternatives: [
                { request_schema: { required: ["size"] }, native_reference_inputs: slots, output_size: fourMP },
                { request_schema: { required: ["size"] }, native_reference_inputs: [slot("reference_image", highLimit, 1)], output_size: eightMP },
            ],
        },
    };
}

test.each([0, 1])("a restored image draft with %i references names the withdrawn output count", async (refCount) => {
    const variant = refCount ? edit() : generate;
    const limited = {
        ...variant,
        specifications_schema: { ...variant.specifications_schema, properties: { ...variant.specifications_schema.properties, "/n": { type: "integer", minimum: 1, maximum: 1 } } },
        native_request_variant: {
            ...variant.native_request_variant,
            supply_alternatives: [fourMP, eightMP].map((output_size) => ({ output_size, native_reference_inputs: variant.native_request_variant.native_reference_inputs, request_schema: { required: ["size"] } })),
        },
    };
    const stale = requests("image", [limited], { "/n": 2 }, images(refCount));
    expect(stale.quote).toBeNull();
    await expect(stale.submit()).rejects.toThrow("生成张数仅支持 1 张");
    expect(JSON.parse(stale.config.nativeParameters)).toEqual({ "/n": 2 });
    for (const parameters of [{}, { "/n": 1 }]) await requests("image", [limited], parameters, images(refCount)).submit();
    await expect(requests("image", [limited], { "/n": 1, "/size": "1025x1024" }, images(refCount)).submit()).rejects.toThrow("16 的整数倍");
});

test.each([
    ["2048x2048", 0, "generate"],
    ["2048x2048", 16, "edit"],
    ["3072x2048", 1, "edit"],
    ["3072x2048", 16, "edit"],
    ["auto", 16, "edit"],
] as const)("GPT image preview and submit choose the same published route for %s with %d references", async (size, count, endpoint) => {
    const explicit = { "/size": size };
    const { quote, submit } = requests("image", [generate, edit()], explicit, images(count));
    const task = await submit();
    expect(task.platform?.variantId).toBe(endpoint);
    expect(quote?.platform?.variantId).toBe(endpoint);
    expect(task.input?.config).toMatchObject({ nativeParameters: explicit, nativeVariantId: endpoint });
    expect(quote?.input?.config).toEqual(task.input?.config);
    expect(task.input?.referenceImages).toHaveLength(count);
});

test.each([
    ["3072x2048", 16, 1, "最多支持 1 张参考图"],
    ["auto", 16, 1, "最多支持 1 张参考图"],
    ["2048x2048", 17, 16, "最多支持 16 张参考图"],
    ["3840x3840", 1, 16, "单张图片像素"],
] as const)("GPT image admission never combines an output size from one supply with input capacity from another (%s, %d refs)", async (size, count, highLimit, issue) => {
    const explicit = { "/size": size };
    const { config, quote, submit } = requests("image", [generate, edit(highLimit)], explicit, images(count));
    expect(quote).toBeNull();
    await expect(submit()).rejects.toThrow(issue);
    expect(JSON.parse(config.nativeParameters)).toEqual(explicit);
});

for (const prefix of ["before/", ""]) {
    test.each([
        [0, "1024x1024", "1536x1024"],
        [1, "1536x1024", "1024x1024"],
    ] as const)(`${prefix}product detail %i resolves the sole executable size for both preview and submit`, async (index, size, invalidSize) => {
        const variants = catalogVariants(`${prefix}studio-detail-partial-${index}`);
        const empty = requests("image", variants, {}, []);
        const expected = { "/size": size };
        expect(empty.quote?.input?.config?.nativeParameters).toEqual(expected);
        expect((await empty.submit()).input?.config?.nativeParameters).toEqual(expected);
        expect(JSON.parse(empty.config.nativeParameters)).toEqual({});
        const stale = requests("image", variants, { "/size": invalidSize }, []);
        expect(stale.quote).toBeNull();
        await expect(stale.submit()).rejects.toThrow();
        expect(JSON.parse(stale.config.nativeParameters)).toEqual({ "/size": invalidSize });
    });
}

test.each([{ "/quality": "high" }, { "/n": 2 }])("the full product detail keeps %o and fills only the size of the matching supply", async (explicit) => {
    const variants = catalogVariants("studio-detail-all");
    const { quote, submit } = requests("image", variants, explicit, []);
    const expected = { ...explicit, "/size": "1536x1024" };
    expect(quote?.input?.config?.nativeParameters).toEqual(expected);
    expect((await submit()).input?.config?.nativeParameters).toEqual(expected);
});

test("two eligible supplies with different required sizes and no common default let the user choose", () => {
    const [variant] = catalogVariants("studio-detail-all");
    const noDefault = { ...variant, defaults: { ...variant.defaults, "/size": "2048x2048" } };
    expect(supplySelection(noDefault, {}).issue).toContain("明确选择");
    expect(supplySelection(noDefault, { "/size": "1536x1024" }).values).toEqual({ "/size": "1536x1024" });
});

test("only required singleton enums supply a missing value, including explicit falsy constants", () => {
    const properties = { "/n": { enum: [0, 1] }, "/audio": { type: "boolean" }, "/optional": { type: "boolean" }, "/nullable": { type: ["string", "null"] } };
    const schema = { required: ["n", "audio", "nullable"], properties: { n: { enum: [0] }, audio: { enum: [false] }, optional: { enum: [false] }, nullable: { enum: [null] } } };
    const variant = {
        ...video,
        defaults: { "/n": 1, "/audio": true, "/optional": false },
        specifications_schema: { properties },
        native_request_variant: { ...video.native_request_variant, supply_alternatives: [{ request_schema: schema, native_reference_inputs: [] }] },
    };
    expect(supplySelection(variant, {})).toMatchObject({ issue: "", values: { "/n": 0, "/audio": false, "/nullable": null } });
    expect(supplySelection(variant, {}).values).not.toHaveProperty("/optional");
    expect(supplySelection(variant, { "/audio": true })).toMatchObject({ values: { "/audio": true } });
    expect(supplySelection(variant, { "/audio": true }).issue).not.toBe("");
});
