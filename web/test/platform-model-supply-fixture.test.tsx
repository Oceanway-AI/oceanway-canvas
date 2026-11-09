import { expect, test } from "bun:test";
import { renderToStaticMarkup } from "react-dom/server";
import projections from "../../backend/internal/app/testdata/platform-model-supply-20261109/studio-projection.json";
import videoEvidence from "../../backend/internal/app/testdata/platform-model-supply-20261109/video-published.json";
import imageEvidence from "../../backend/internal/app/testdata/platform-model-supply-20261109/image-published.json";
import { systemChannelModelChannels } from "../src/lib/user-session";
import { VideoSettingsPanel } from "../src/components/video-settings-panel";
import { canvasThemes } from "../src/lib/canvas-theme";
import { videoDurationOptions } from "../src/lib/model-capabilities";
import { defaultConfig } from "../src/stores/use-config-store";
import { creationVideoConfig } from "../src/pages/create/creation-generation-config";
import { prepareBackendGenerationTask } from "../src/services/api/generation-task";
import { submitPlatformTask } from "../src/services/platform-task-submission";
import type { PublicChannelModel } from "../src/services/api/logical-models";
import type { GenerationTask } from "../src/services/api/task-center";

const channels = systemChannelModelChannels([{ id: "oceanway-platform", name: "OceanWay", displayName: "OceanWay", models: projections as PublicChannelModel[] }]);

test("actual provider publication through Studio projection keeps one variant and no pricing inputs", async () => {
    const model = projections.find((m) => m.capability === "video") as PublicChannelModel;
    const profile = model.capabilityConfig!.video!;
    expect(videoDurationOptions(profile)).toEqual([6, 9, 12, 15]);
    expect(profile.resolutions).toEqual(["720p"]);
    expect(profile.generateAudio).toEqual({ supported: true, default: false });
    const selected = `oceanway-platform::${model.modelKey}`;
    const config = creationVideoConfig({ ...defaultConfig, channels }, selected, { seconds: "6", ratio: "", videoQuality: "720" });
    const html = renderToStaticMarkup(<VideoSettingsPanel config={config} theme={canvasThemes.dark} onConfigChange={() => {}} />);
    expect(html).toContain('min="6"');
    expect(html).toContain('step="3"');
    expect(html).not.toContain("1080P");
    expect(html).not.toContain("自动同步可信素材");
    const request = await prepareBackendGenerationTask({ mode: "video", prompt: "free frozen contract fixture", config, clientOperationId: "provider-fixture-command" });
    expect(request.input?.config).toMatchObject({ model: model.modelKey, videoSeconds: "6", vquality: "720p" });
    expect(request.input?.config).not.toHaveProperty("tokens");
    expect(request.input?.config).not.toHaveProperty("video_input");
    let creates = 0;
    let recoveries = 0;
    const task = await submitPlatformTask(request, {
        scope: () => "fixture-user",
        preview: async () => videoEvidence.preview,
        confirmBudget: async () => "1",
        create: async (frozen) => {
            creates++;
            expect(frozen.platform).toEqual({
                expectedPriceVersion: videoEvidence.preview.price_version,
                expectedCapabilityRevision: videoEvidence.preview.capability_revision,
                expectedConnectionRevision: videoEvidence.preview.connection_revision,
                walletMaxPoints: "1",
            });
            expect(frozen.input).toEqual(request.input);
            throw new Error("lost response");
        },
        recover: async (command) => {
            recoveries++;
            expect(command).toBe("provider-fixture-command");
            return { id: "same-task" } as GenerationTask;
        },
    });
    expect(task.id).toBe("same-task");
    expect([creates, recoveries]).toEqual([1, 1]);
});

test("provider image 200 preview with unverified outputs creates no task", async () => {
    const model = projections.find((m) => m.capability === "image") as PublicChannelModel;
    expect(model.capabilityConfig!.image!.size.values).toEqual(["1024x1024"]);
    expect(model.capabilityConfig!.image!.parameters?.map((p) => p.name)).not.toContain("images");
    const selected = `oceanway-platform::${model.modelKey}`;
    const config = { ...defaultConfig, channels, model: selected, imageModel: selected, size: "1024x1024", count: "1" };
    const request = await prepareBackendGenerationTask({ mode: "image", prompt: "free fixture", config, clientOperationId: "provider-image-command" });
    let creates = 0;
    let recoveries = 0;
    let rejected = false;
    try {
        await submitPlatformTask(request, {
            scope: () => "fixture-user",
            preview: async () => imageEvidence.preview,
            create: async () => {
                creates++;
                return { id: "unexpected" } as GenerationTask;
            },
            recover: async () => {
                recoveries++;
                return { id: "unexpected" } as GenerationTask;
            },
        });
    } catch {
        rejected = true;
    }
    expect(rejected).toBe(true);
    expect([creates, recoveries]).toEqual([0, 0]);
});
