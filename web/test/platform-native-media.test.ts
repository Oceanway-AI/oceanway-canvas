import { expect, test } from "bun:test";
import { defaultConfig } from "@/stores/use-config-store";
import { systemChannelModelChannels } from "@/lib/user-session";
import { nativeMediaDefaults, nativeMediaReferenceLimits, nativeMediaVariant, nativeParameterValues } from "@/lib/platform-native-media";
import { modelCompatibilityError, resolveModelGenerationDefaults } from "@/lib/model-selection";
import { backendProviderConfig } from "@/services/api/generation-task";
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
const model = {
    id: "native",
    modelKey: "doubao-seedance-2-0-fast-260128",
    displayName: "Seedance",
    capability: "video",
    protocol: "volcengine-ark-video",
    available: true,
    priceTiers: [],
    platform: {
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

test("selected protocol owns reference capacity without merging alternatives", () => {
    expect(nativeMediaReferenceLimits({ ...config, nativeVariantId: "ark" })?.maxImages).toBe(2);
    expect(nativeMediaReferenceLimits({ ...config, nativeVariantId: "other" })?.maxImages).toBe(0);
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
