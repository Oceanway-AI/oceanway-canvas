import { expect, test } from "bun:test";
import { buildNodeGenerationContext } from "@/components/canvas/canvas-node-generation";
import { buildNodeConfig, nativeCanvasParameterPatch } from "@/components/canvas/canvas-node-prompt-config";
import { createCanvasBatchInputSnapshot } from "@/lib/canvas/canvas-batch-input-snapshot";
import { applyNodeConfigPatch } from "@/lib/canvas/canvas-project-domain";
import { buildGenerationConfig, buildVideoGenerationMetadata } from "@/lib/canvas/canvas-project-generation";
import { generationSpecMetadata, specFromConfig } from "@/lib/canvas/generation-contract";
import { modelCompatibilityError } from "@/lib/model-selection";
import { systemChannelModelChannels } from "@/lib/user-session";
import { prepareBackendGenerationTask } from "@/services/api/generation-task";
import type { PublicChannelModel } from "@/services/api/logical-models";
import { defaultConfig } from "@/stores/use-config-store";
import { CanvasNodeType, type CanvasNodeData } from "@/types/canvas";

const ratios = ["adaptive", "1:1", "4:3", "3:4", "16:9", "9:16", "21:9"];
function videoVariant(frames: boolean) {
    return {
        id: frames ? "frames" : "text",
        gateway_protocol: "modelark.video",
        workbench_parameters: ["/ratio", "/generate_audio", "/resolution"],
        defaults: { "/ratio": frames ? "adaptive" : "16:9", "/generate_audio": true, "/resolution": "720p" },
        specifications_schema: { properties: { "/ratio": frames ? { const: "adaptive" } : { enum: ratios }, "/generate_audio": frames ? { const: true } : { type: "boolean" }, "/resolution": { enum: ["480p", "720p"] } } },
        native_request_variant: {
            protocol: "modelark.video",
            path: "/video",
            modes: ["async"],
            request_schema: {},
            native_reference_inputs: frames ? ["first_frame", "last_frame"].map((role) => ({ id: role, pointer: "/content", transport: "content-array", role, min_items: role === "first_frame" ? 1 : 0, max_items: 1 })) : [],
        },
    };
}
const published = (modelKey: string) =>
    ({
        id: modelKey,
        modelKey,
        displayName: "同名视频",
        capability: "video",
        protocol: "volcengine-ark-video",
        available: true,
        priceTiers: [],
        platform: { model_type: "video", price: { version: "test-price" }, media_generation_profile: { gateway_protocol: "modelark.video", default_variant: "text", variants: [videoVariant(false), videoVariant(true)] } },
    }) as unknown as PublicChannelModel;
const channels = systemChannelModelChannels([{ id: "oceanway-platform", name: "Studio", displayName: "Studio", models: [published("other"), published("selected")] }]);
channels[0].modelCosts![1].logicalModelId = "saved-logical";
const config = { ...defaultConfig, model: "oceanway-platform::other", videoModel: "oceanway-platform::other", channels };
const empty = { textCount: 1, imageCount: 0, videoCount: 0, audioCount: 0, characterCount: 0 };
function makeNode(id: string, type: CanvasNodeType, metadata: CanvasNodeData["metadata"]): CanvasNodeData {
    return { id, type, title: id, position: { x: 0, y: 0 }, width: 220, height: 150, metadata };
}

test.each([CanvasNodeType.Video, CanvasNodeType.Config])("%s keeps its model and frame slots as empty and connected requests switch contracts", async (type) => {
    const selected = { ...config, model: "oceanway-platform::selected", nativeVariantId: "frames" };
    const metadata = type === CanvasNodeType.Video ? generationSpecMetadata(specFromConfig("video", "镜头", selected)) : { model: selected.model, generationMode: "video" as const, nativeVariantId: "frames", composerContent: "镜头" };
    let node = makeNode("target", type, metadata);
    const editor = buildNodeConfig(config, node, "video", { capability: "video", input: empty });
    expect(editor.model).toBe(selected.model);
    expect(editor.nativeVariantId).toBe("text");
    const values = { "/ratio": "9:16", "/generate_audio": false, "/resolution": "480p" };
    node = applyNodeConfigPatch(node, nativeCanvasParameterPatch(editor, "video", "frames", "nativeParameters", JSON.stringify(values)));
    node = JSON.parse(JSON.stringify(node));
    const draft = buildGenerationConfig(config, node, "video", { capability: "video", input: empty });
    const textTask = await prepareBackendGenerationTask({ mode: "video", prompt: "镜头", config: draft });
    expect(textTask.input?.config).toMatchObject({ model: "selected", nativeParameters: values, nativeVariantId: "text" });

    const last = makeNode("tail", CanvasNodeType.Image, { storageKey: "resource:tail" });
    const first = makeNode("first", CanvasNodeType.Image, { storageKey: "resource:first" });
    node = applyNodeConfigPatch(node, { videoEndFrameNodeId: last.id });
    const tailContext = buildNodeGenerationContext(node.id, [last, node], [{ id: "tail-edge", fromNodeId: last.id, toNodeId: node.id }], "镜头", [], true);
    const tailConfig = buildGenerationConfig(config, node, "video", { capability: "video", input: { ...empty, imageCount: 1 } });
    expect(() => buildVideoGenerationMetadata(node, tailContext, tailConfig)).toThrow("请添加首帧图片");
    node = applyNodeConfigPatch(node, { videoStartFrameNodeId: first.id });
    if (type === CanvasNodeType.Video) {
        // Persisted contracts may contain only the bindings without flat frame mirrors.
        node = { ...node, metadata: { generationSpec: node.metadata?.generationSpec, nativeModeVariantId: node.metadata?.nativeModeVariantId } };
    }
    const context = buildNodeGenerationContext(
        node.id,
        [last, first, node],
        [last, first].map((n) => ({ id: "edge-" + n.id, fromNodeId: n.id, toNodeId: node.id })),
        "镜头",
        [],
        true,
    );
    const input = { ...empty, imageCount: context.imageCount };
    const requestConfig = buildGenerationConfig(config, node, "video", { capability: "video", input });
    expect(buildNodeConfig(config, node, "video", { capability: "video", input }).nativeParameters).toBe(requestConfig.nativeParameters);
    const request = await prepareBackendGenerationTask({ mode: "video", prompt: "镜头", config: requestConfig, referenceImages: context.referenceImages, metadata: buildVideoGenerationMetadata(node, context, requestConfig) });
    expect(request.input?.config).toMatchObject({ model: "selected", nativeVariantId: "frames", nativeParameters: { "/ratio": "adaptive", "/generate_audio": true, "/resolution": "480p" } });
    expect(request.input?.referenceImages).toMatchObject([
        { storageKey: "resource:first", role: "first_frame" },
        { storageKey: "resource:tail", role: "last_frame" },
    ]);
    const explicitText = applyNodeConfigPatch(node, nativeCanvasParameterPatch(requestConfig, "video", "frames", "nativeVariantId", "text"));
    const incompatible = buildGenerationConfig(config, explicitText, "video", { capability: "video", input });
    expect(incompatible.nativeVariantId).toBe("text");
    expect(modelCompatibilityError(incompatible, incompatible.model, { capability: "video", input })).not.toBe("");
});

test("an image configuration with a plain prompt includes attached media in the immediate and saved batch input", () => {
    const node = makeNode("edit", CanvasNodeType.Config, { nativeVariantId: "edit", composerContent: "换成蓝色", generationMode: "image" });
    const photo = makeNode("image", CanvasNodeType.Image, { storageKey: "resource:ocean" });
    const edges = [{ id: "edge", fromNodeId: photo.id, toNodeId: node.id }];
    const context = buildNodeGenerationContext(node.id, [node, photo], edges, "换成蓝色", [], false, true);
    expect(context.referenceImages).toMatchObject([{ id: photo.id, storageKey: "resource:ocean" }]);
    expect(createCanvasBatchInputSnapshot(node, [node, photo], edges, []).context.referenceImages).toMatchObject([{ id: photo.id, storageKey: "resource:ocean" }]);
});
