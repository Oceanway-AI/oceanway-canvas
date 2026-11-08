import { expect, test } from "bun:test";
import { defaultConfig } from "@/stores/use-config-store";
import { defaultModelCapabilityConfig } from "@/lib/model-capabilities";
import { systemChannelModelChannels } from "@/lib/user-session";
import { creationVideoConfig } from "@/pages/create/creation-generation-config";
import { prepareBackendGenerationTask } from "@/services/api/generation-task";
import { submitPlatformTask } from "@/services/platform-task-submission";
import type { PublicChannelModel } from "@/services/api/logical-models";
import type { GenerationTask } from "@/services/api/task-center";

const platform = {
    model_name: "doubao-seedance-2-0-fast-260128",
    model_type: "video",
    price: { status: "usage_dependent", version: "new-token-price", rates: {}, usage_schema: { tokens: { type: "number", unit: "token" }, resolution: { enum: ["480p", "720p"] }, video_input: { enum: ["none", "video"] } } },
};
const model = {
    id: "video",
    modelKey: platform.model_name,
    displayName: "Seedance",
    protocol: "volcengine-ark-video",
    capability: "video",
    available: true,
    priceTiers: [],
    platform,
    capabilityConfig: {
        version: 1,
        video: {
            ...defaultModelCapabilityConfig().video!,
            ratios: ["16:9", "1:1", "adaptive"],
            defaultRatio: "adaptive",
            resolutions: ["480p", "720p"],
            defaultResolution: "720p",
            duration: { selection: "enum", values: [-1, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15], default: 0 },
        },
    },
} as PublicChannelModel;

test("直接生成把内部720还原为发布值720p，保留4秒、新价格版本和用量合同", async () => {
    const channels = systemChannelModelChannels([{ id: "oceanway-platform", name: "OceanWay", displayName: "OceanWay", models: [model] }]);
    const selected = `oceanway-platform::${platform.model_name}`;
    const config = creationVideoConfig({ ...defaultConfig, channels }, selected, { ratio: "16:9", seconds: "4", videoQuality: "720" });
    const request = await prepareBackendGenerationTask({ mode: "video", prompt: "A quiet ocean", config, clientOperationId: "native-video-command" });
    expect(channels[0].modelCosts![0].platform?.price).toEqual(platform.price);
    expect(request.platform?.expectedPriceVersion).toBe("new-token-price");
    expect(request.input?.config).toMatchObject({ model: platform.model_name, vquality: "720p", videoSeconds: "4", size: "16:9" });
    expect(request.input?.config).not.toHaveProperty("tokens");
    let previews = 0,
        submits = 0,
        recoveries = 0;
    const result = await submitPlatformTask(request, {
        scope: () => "alice",
        preview: async (input) => {
            previews++;
            expect(input.input).toEqual(request.input);
            return { capability_revision: "a".repeat(64), connection_revision: 7, price_version: "new-token-price", wallet_points: "0.123456789", allowed: true, combination_supported: true };
        },
        create: async (input) => {
            submits++;
            expect(input.platform).toEqual({ expectedCapabilityRevision: "a".repeat(64), expectedConnectionRevision: 7, expectedPriceVersion: "new-token-price", walletMaxPoints: "0.123456789" });
            expect(input.input).toEqual(request.input);
            throw new Error("lost original response");
        },
        recover: async (command) => {
            recoveries++;
            expect(command).toBe("native-video-command");
            return { id: "original-video" } as GenerationTask;
        },
    });
    expect(result.id).toBe("original-video");
    expect([previews, submits, recoveries]).toEqual([1, 1, 1]);
});
