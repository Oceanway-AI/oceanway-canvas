import { expect, test } from "bun:test";
import { defaultConfig } from "@/stores/use-config-store";
import { systemChannelModelChannels } from "@/lib/user-session";
import { nativeCreationModeOptions, nativeCreationSettings, nativeMediaDefaults, nativeMediaReferenceLimits, nativeMediaVariant, nativeMediaVariantOptions, nativeParameterValues } from "@/lib/platform-native-media";
import { maxModelInputCapacity, modelCompatibilityError, resolveModelGenerationDefaults } from "@/lib/model-selection";
import { studioMediaPriceRequest } from "@/services/studio-media-price";
import { backendProviderConfig, prepareBackendGenerationTask, runBackendGenerationTaskBatch } from "@/services/api/generation-task";
import { buildNodeConfig } from "@/components/canvas/canvas-node-prompt-config";
import { buildGenerationConfig } from "@/lib/canvas/canvas-project-generation";
import { specFromConfig, generationSpecMetadata, synchronizeGenerationSpec } from "@/lib/canvas/generation-contract";
import { normalizeCreationComposerPreferences } from "@/stores/use-creation-preferences-store";
import { agentMediaSettings, agentApprovalMatchesSettings } from "@/lib/canvas/agent-media-approval";
import type { PublicChannelModel } from "@/services/api/logical-models";
import { CanvasNodeType, type CanvasNodeData } from "@/types/canvas";

const defaults = { "/duration": 4, "/resolution": "720p", "/seed": 8, "/generate_audio": true, "/metadata": { source: "test" }, "/extensions": ["IMAGE"] };
const properties = Object.fromEntries(Object.keys(defaults).map((key) => [key, { type: key === "/duration" || key === "/seed" ? "integer" : "string" }]));
const variant = {
    id: "ark",
    gateway_protocol: "modelark.video",
    defaults,
    specifications_schema: { properties },
    native_request_variant: {
        protocol: "modelark.video",
        path: "/api/v3/contents/generations/tasks",
        modes: ["async"],
        request_schema: {},
        native_reference_inputs: [
            { id: "first", pointer: "/content", transport: "content-array", role: "first_frame", max_items: 1 },
            { id: "last", pointer: "/content", transport: "content-array", role: "last_frame", max_items: 1 },
        ],
    },
};

test("native protocol choices describe the published reference inputs without model-name inference", () => {
    const text = { ...variant, id: "text", native_request_variant: { ...variant.native_request_variant, native_reference_inputs: [] } };
    const frames = {
        ...variant,
        id: "frames",
        native_request_variant: { ...variant.native_request_variant, native_reference_inputs: variant.native_request_variant.native_reference_inputs.map((ref) => ({ ...ref, min_items: ref.role === "first_frame" ? 1 : 0 })) },
    };
    const multi = {
        ...variant,
        id: "multi",
        native_request_variant: {
            ...variant.native_request_variant,
            native_reference_inputs: [
                { id: "images", pointer: "/content", transport: "content-array", role: "reference_image", min_items: 0, max_items: 9 },
                { id: "video", pointer: "/content", transport: "content-array", role: "reference_video", min_items: 0, max_items: 3 },
            ],
        },
    };
    const options = nativeMediaVariantOptions([text, frames, multi]);
    expect(options).toEqual([
        { value: "text", label: "modelark.video · 文生视频" },
        { value: "frames", label: "modelark.video · 首帧（必填） + 尾帧（可选）" },
        { value: "multi", label: "modelark.video · 参考图片（可选，最多 9 个） + 参考视频（可选，最多 3 个）" },
    ]);
    expect(nativeMediaVariantOptions([text, { ...text, id: "other" }]).map((option) => option.label)).toEqual(["modelark.video · 文生视频 · text", "modelark.video · 文生视频 · other"]);
});
test("creation mode choices hide duplicate protocols and keep the selected native route", () => {
    const variants = ["sora.video", "newapi.video", "modelark.video"].flatMap((protocol) =>
        [[], ["first_frame", "last_frame"], ["reference_image", "reference_video", "reference_audio"]].map((roles, i) => ({
            ...variant,
            id: protocol + i,
            gateway_protocol: protocol,
            native_request_variant: { ...variant.native_request_variant, native_reference_inputs: roles.map((role) => ({ ...variant.native_request_variant.native_reference_inputs[0], role })) },
        })),
    );
    expect(nativeCreationModeOptions(variants, variants[6], true)).toEqual([
        { label: "首尾帧", value: "modelark.video1" },
        { label: "全能参考", value: "modelark.video2" },
    ]);
});
const model = {
    id: "native",
    modelKey: "doubao-seedance-2-0-fast-260128",
    displayName: "Seedance",
    capability: "video",
    protocol: "volcengine-ark-video",
    available: true,

    platform: {
        price: { version: "fixture" },
        model_type: "video",
        media_generation_profile: {
            gateway_protocol: variant.gateway_protocol,
            native_request_variant: variant.native_request_variant,
            default_variant: "ark",
            variants: [variant, { ...variant, id: "other", defaults: { "/duration": 10 }, native_request_variant: { ...variant.native_request_variant, native_reference_inputs: [] } }],
        },
    },
    capabilityConfig: { version: 1, video: { references: { maxImages: 0 } } },
} as unknown as PublicChannelModel;
const channels = systemChannelModelChannels([{ id: "oceanway-platform", name: "OceanWay", displayName: "OceanWay", models: [model] }]);
const selected = "oceanway-platform::" + model.modelKey;
const config = { ...defaultConfig, channels, model: selected, videoModel: selected };
const explicit = { "/duration": 4, "/resolution": "720p", "/seed": 0, "/generate_audio": false, "/metadata": null, "/extensions": ["TEXT", "IMAGE"] };

test("summary models remain discoverable but cannot submit without complete detail", async () => {
    const summary = { ...model, platform: { ...model.platform!, state: "summary" as const, model_name: model.modelKey, media_generation_profile: undefined, variants_complete: false, variants: [] } };
    const active = { ...config, channels: systemChannelModelChannels([{ id: "oceanway-platform", name: "OceanWay", displayName: "OceanWay", models: [summary] }]) };
    expect(modelCompatibilityError(active, selected, { capability: "video" })).toBe("");
    expect(resolveModelGenerationDefaults(active, selected, "video", { nativeParameters: '{"/duration":8}', nativeVariantId: "saved" })).toEqual({ nativeParameters: '{"/duration":8}', nativeVariantId: "saved" });
    await expect(prepareBackendGenerationTask({ mode: "video", config: active, prompt: "A scene" })).rejects.toThrow("模型参数尚未加载");
});

test("loaded detail pins capability and connection versions in task admission", async () => {
    const loaded = { ...model, platform: { ...model.platform!, state: "detail" as const, capability_revision: "capability-v2", connection_revision: 7 } };
    const active = { ...config, channels: systemChannelModelChannels([{ id: "oceanway-platform", name: "OceanWay", displayName: "OceanWay", models: [loaded] }]) };
    const request = await prepareBackendGenerationTask({ mode: "video", config: active, prompt: "A scene" });
    expect(request.platform).toMatchObject({ expectedPriceVersion: "fixture", expectedCapabilityRevision: "capability-v2", expectedConnectionRevision: 7 });
});

test("task inputs preserve an explicitly picked tail frame, canvas frame bindings and ordered legacy attachments", async () => {
    const frame = (id: string) => ({ id, name: id, type: "image/png", dataUrl: "", storageKey: "resource:" + id });
    const first = frame("first");
    const last = frame("last");
    const options = { mode: "video" as const, prompt: "ocean", config: { ...config, nativeVariantId: "ark" } };
    const tailOnly = await prepareBackendGenerationTask({ ...options, referenceImages: [{ ...last, role: "last_frame" }] });
    expect(tailOnly.input?.referenceImages).toMatchObject([{ id: "last", role: "last_frame" }]);
    const reversed = await prepareBackendGenerationTask({ ...options, referenceImages: [{ ...last, role: "reference" }, first], metadata: { videoEndFrameNodeId: last.id } });
    expect(reversed.input?.referenceImages).toMatchObject([
        { id: "first", role: "first_frame" },
        { id: "last", role: "last_frame" },
    ]);
    const legacy = await prepareBackendGenerationTask({ ...options, referenceImages: [first, last] });
    expect(legacy.input?.referenceImages).toEqual(reversed.input?.referenceImages);
});

test("native selection preserves exact values and ignores legacy video controls", () => {
    const settings = nativeMediaDefaults(config, selected, { nativeVariantId: "ark", nativeParameters: JSON.stringify(explicit) });
    const value = { ...config, ...settings, videoSeconds: "999", vquality: "invented", size: "legacy" };
    expect(nativeParameterValues(nativeMediaVariant(value)!, settings.nativeParameters)).toEqual(explicit);
    expect(backendProviderConfig(value, "video")).toEqual({ channelId: "oceanway-platform", model: model.modelKey, nativeVariantId: "ark", nativeParameters: explicit });
    expect(modelCompatibilityError(value, selected, { capability: "video", videoSeconds: "999", options: { vquality: "invented" } })).toBe("");
    expect(resolveModelGenerationDefaults(value, selected, "video", settings).count).toBe("1");
    expect(() => backendProviderConfig({ ...value, nativeParameters: "[" }, "video")).toThrow();
    expect(() => backendProviderConfig({ ...value, nativeVariantId: "retired" }, "video")).toThrow();
});

test("reopening an editable draft restores a published mode and its own defaults", () => {
    const empty = nativeCreationSettings(config, selected, { nativeVariantId: "retired", nativeParameters: '{"/seed":999}' }, false);
    expect(empty.referenceVariant?.id).toBe("ark");
    expect(empty.settings.nativeVariantId).toBe("other");
    expect(JSON.parse(empty.settings.nativeParameters)["/seed"]).not.toBe(999);
    expect(nativeCreationSettings(config, selected, { nativeVariantId: "other" }, true).settings.nativeVariantId).toBe("ark");
});

test("selected protocol owns reference capacity without merging alternatives", () => {
    expect(maxModelInputCapacity(config, "video", "image")).toBe(2);
    expect(maxModelInputCapacity(config, "video", "audio")).toBe(0);
    expect(nativeMediaReferenceLimits({ ...config, nativeVariantId: "ark" })?.maxImages).toBe(2);
    expect(nativeMediaReferenceLimits({ ...config, nativeVariantId: "other" })?.maxImages).toBe(0);
});

test("video draft keeps the selected reference mode while controls and submission follow the effective request", () => {
    const frames = {
        ...variant,
        defaults: { "/ratio": "adaptive", "/duration": 4 },
        specifications_schema: { properties: { "/ratio": { const: "adaptive" }, "/duration": { type: "integer", minimum: 4, maximum: 10, multipleOf: 2 } } },
        workbench_parameters: ["/ratio", "/duration"],
    };
    const text = {
        ...frames,
        id: "text",
        defaults: { "/ratio": "16:9", "/duration": 5, "/generate_audio": true },
        specifications_schema: { properties: { "/ratio": { enum: ["16:9", "9:16"] }, "/duration": { enum: [5, 6, 15] }, "/generate_audio": { type: "boolean" } } },
        workbench_parameters: ["/ratio", "/duration", "/generate_audio"],
        native_request_variant: { ...frames.native_request_variant, native_reference_inputs: [] },
    };
    const video = { ...model, platform: { ...model.platform, media_generation_profile: { gateway_protocol: frames.gateway_protocol, default_variant: frames.id, variants: [frames, text] } } } as PublicChannelModel;
    const cfg = { ...config, channels: systemChannelModelChannels([{ id: "oceanway-platform", name: "OW", displayName: "OW", models: [video] }]) };
    const initial = nativeCreationSettings(cfg, selected, {}, false);
    expect(JSON.parse(initial.settings.nativeParameters)).toEqual(text.defaults);
    const edited = { nativeModeVariantId: frames.id, nativeVariantId: text.id, nativeParameters: JSON.stringify({ "/ratio": "9:16", "/duration": 6, "/generate_audio": false }) };
    const saved = normalizeCreationComposerPreferences({ video: edited }).video!;
    const empty = nativeCreationSettings(cfg, selected, saved, false);
    expect(empty.referenceVariant?.id).toBe(frames.id);
    expect(backendProviderConfig({ ...cfg, ...empty.settings }, "video")).toMatchObject({ nativeVariantId: text.id, nativeParameters: JSON.parse(edited.nativeParameters) });
    const attached = nativeCreationSettings(cfg, selected, saved, true);
    expect(backendProviderConfig({ ...cfg, ...attached.settings }, "video")).toMatchObject({ nativeVariantId: frames.id, nativeParameters: { "/ratio": "adaptive", "/duration": 6 } });
    expect(JSON.parse(nativeCreationSettings(cfg, selected, { ...saved, nativeParameters: '{"/duration":5}' }, true).settings.nativeParameters)).toEqual(frames.defaults);
});

test("saved canvas, editor and executor submit the same native parameters", () => {
    const configured = { ...config, nativeVariantId: "ark", nativeParameters: JSON.stringify(explicit) };
    const spec = specFromConfig("video", "ocean", configured, []);
    const node = { id: "node", type: CanvasNodeType.Video, position: { x: 0, y: 0 }, metadata: generationSpecMetadata(spec) } as CanvasNodeData;
    const restored = JSON.parse(JSON.stringify(node)) as CanvasNodeData;
    const panel = buildNodeConfig(config, restored, "video", { capability: "video" });
    const execution = buildGenerationConfig(config, restored, "video", { capability: "video" });
    expect(backendProviderConfig(panel, "video")).toEqual(backendProviderConfig(execution, "video"));
    expect(backendProviderConfig(execution, "video").nativeParameters).toEqual(explicit);
    const changed = synchronizeGenerationSpec(restored, { nativeVariantId: "other" });
    expect(changed.metadata?.generationSpec?.options.nativeParameters).toBeUndefined();
});

test("creation history preferences and Agent approval retain native values", () => {
    const settings = { model: selected, nativeVariantId: "ark", nativeParameters: JSON.stringify(explicit) };
    expect(normalizeCreationComposerPreferences({ image: settings, video: settings })).toEqual({ image: settings, video: settings });
    const agent = { channelId: "oceanway-platform", channelModelKey: model.modelKey, nativeVariantId: settings.nativeVariantId, nativeParameters: settings.nativeParameters };
    expect(agentMediaSettings(agent)).toMatchObject(agent);
    expect(agentApprovalMatchesSettings(agent, { ...agent, nativeParameters: "{}" })).toBe(false);
});

test("creation selects a published edit only for attached images and retains compatible settings", () => {
    const generate = {
        ...variant,
        id: "generate",
        gateway_protocol: "openai.image.generate",
        defaults: { "/size": "auto", "/quality": "low" },
        specifications_schema: { properties: { "/size": { type: "string" }, "/quality": { enum: ["low", "high"] } } },
        native_request_variant: { ...variant.native_request_variant, native_reference_inputs: [] },
    };
    const edit = {
        ...generate,
        id: "edit",
        gateway_protocol: "openai.image.edit",
        defaults: { "/size": "auto", "/quality": "medium" },
        specifications_schema: { properties: { "/size": { type: "string" }, "/quality": { enum: ["medium"] } } },
        native_request_variant: { ...variant.native_request_variant, native_reference_inputs: [{ id: "ref", pointer: "/image", transport: "image-reference-array", role: "reference_image", min_items: 1, max_items: 16 }] },
    };
    const image = {
        ...model,
        capability: "image",
        platform: { ...model.platform, model_type: "image", media_generation_profile: { gateway_protocol: generate.gateway_protocol, default_variant: generate.id, variants: [generate, edit] } },
    } as PublicChannelModel;
    const cfg = { ...config, channels: systemChannelModelChannels([{ id: "oceanway-platform", name: "OW", displayName: "OW", models: [image] }]) };
    const saved = { nativeVariantId: "generate", nativeParameters: JSON.stringify({ "/size": "1024x1024", "/quality": "high" }) };
    const empty = nativeCreationSettings(cfg, selected, saved, false);
    expect(empty.settings.nativeVariantId).toBe("generate");
    expect(nativeMediaReferenceLimits(cfg, selected, empty.referenceVariant)?.maxImages).toBe(16);
    const attached = nativeCreationSettings(cfg, selected, saved, true);
    expect(attached.settings.nativeVariantId).toBe("edit");
    expect(JSON.parse(attached.settings.nativeParameters)).toEqual({ "/size": "1024x1024", "/quality": "medium" });
    const input = { textCount: 1, imageCount: 1, videoCount: 0, audioCount: 0, characterCount: 0 };
    expect(modelCompatibilityError(cfg, selected, { capability: "image", input, useImageEditForReferences: true })).toBe("");
    expect(modelCompatibilityError(cfg, selected, { capability: "image", input })).toContain("0");
    expect(modelCompatibilityError(cfg, selected, { capability: "image", input: { ...input, imageCount: 17 }, useImageEditForReferences: true })).toContain("16");
    expect(nativeCreationSettings(cfg, selected, { nativeVariantId: "retired" }, true).settings.nativeVariantId).toBe("edit");
    const cleared = nativeCreationSettings(cfg, selected, { nativeVariantId: "edit" }, false);
    expect(nativeCreationModeOptions([generate, edit], edit, true)).toEqual([{ label: "图片参考", value: "edit" }]);
    expect(cleared.referenceVariant?.id).toBe("edit");
    expect(cleared.settings.nativeVariantId).toBe("generate");
    image.platform!.media_generation_profile!.variants = [generate, edit, { ...edit, id: "another" }];
    expect(nativeCreationSettings(cfg, selected, saved, true).settings.nativeVariantId).toBe("generate");
});

test("Platform controls discard API-only and stale overrides while retaining explicit zero", () => {
    const variant = nativeMediaVariant(config)!;
    const source = { ...variant, workbench_parameters: ["/seed"], defaults: { "/seed": 3, "/n": 1 } };
    expect(nativeParameterValues(source, '{"/seed":0,"/n":8}')).toEqual({ "/seed": 0 });
    expect(nativeParameterValues({ ...source, workbench_parameters: [] }, '{"/n":8}')).toEqual({});
});

test("one published v1 image command owns all selected outputs even without native editor details", async () => {
    const image = {
        ...model,
        modelKey: "fixture-image",
        capability: "image",
        platform: { model_type: "image", price: { version: "p" }, media_generation_profile: { default_variant: "generate", variants: [{ id: "generate", media_generation_v1: true }, { id: "other" }] } },
        capabilityConfig: { version: 1, image: { maxOutputs: 3 } },
    } as unknown as PublicChannelModel;
    const name = "oceanway-platform::fixture-image";
    const cfg = { ...defaultConfig, channels: systemChannelModelChannels([{ id: "oceanway-platform", name: "Platform", displayName: "Platform", models: [image] }]), model: name, imageModel: name, count: "3" };
    const calls: Array<Awaited<ReturnType<typeof prepareBackendGenerationTask>>> = [];
    const result = await runBackendGenerationTaskBatch(
        { mode: "image", prompt: "fixture", config: cfg, count: 3 },
        {
            createTask: async (input) => {
                calls.push(input);
                throw new Error("test stops at admission");
            },
            waitTask: async () => {
                throw new Error("unexpected wait");
            },
            createId: () => "command",
        },
    );
    expect(result).toHaveLength(1);
    expect(calls).toHaveLength(1);
    expect(calls[0]?.platform).toMatchObject({ mediaGenerationV1: true, variantId: "generate" });
    expect(calls[0]?.input?.config).toMatchObject({ count: "3" });
    expect((await prepareBackendGenerationTask({ mode: "image", prompt: "fixture", config: { ...cfg, nativeVariantId: "other" } })).platform?.mediaGenerationV1).toBeUndefined();
});

test("standard Studio estimates use only the current original pins and the same published native parameters as submission", async () => {
    const marked = { ...model, platform: { ...model.platform!, media_generation_profile: { ...model.platform!.media_generation_profile, variants: [{ ...variant, media_generation_v1: true }] } } };
    const active = { ...config, channels: systemChannelModelChannels([{ id: "oceanway-platform", name: "OceanWay", displayName: "OceanWay", models: [marked] }]), nativeVariantId: variant.id, nativeParameters: '{"/duration":4}' };
    const media = [
        { id: "end", name: "end", type: "image/png", dataUrl: "", storageKey: "resource:end", role: "last_frame" as const },
        { id: "first", name: "first", type: "image/png", dataUrl: "", storageKey: "resource:first", role: "first_frame" as const },
    ];
    const refs = media.map((pin) => ({ kind: "image", storageKey: pin.storageKey, role: pin.role }));
    const original = await prepareBackendGenerationTask({ mode: "video", config: active, prompt: "A new scene", referenceImages: media });
    const estimate = studioMediaPriceRequest(active, "video", "A new scene", refs, "current-owner", "project");
    expect(estimate?.input?.config).toEqual(original.input?.config);
    expect(estimate?.input?.referenceImages).toEqual([
        { storageKey: "resource:first", role: "first_frame" },
        { storageKey: "resource:end", role: "last_frame" },
    ]);
    expect(estimate?.platform).toEqual({ mediaGenerationV1: true, expectedUserId: "current-owner", variantId: variant.id, expectedPriceVersion: "fixture" });
    expect(estimate?.commandId).toBeUndefined();
    expect(studioMediaPriceRequest(active, "video", "A new scene", [{ kind: "video", storageKey: "blob:temporary" }], "current-owner")).toBeNull();
    expect(studioMediaPriceRequest(active, "video", "A new scene", refs, "")).toBeNull();
    expect(studioMediaPriceRequest(config, "video", "A new scene", refs, "current-owner")).toBeNull();
});
