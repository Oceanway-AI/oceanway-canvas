import { describe, expect, test } from "bun:test";
import { renderToStaticMarkup } from "react-dom/server";
import { ImageSettingsPanel } from "../src/components/image-settings-panel";
import { canvasThemes } from "../src/lib/canvas-theme";
import { defaultImageCapabilityConfig } from "../src/lib/model-capabilities";

import type { AiConfig } from "../src/stores/use-config-store";

function seedreamConfig(model: string): AiConfig {
    return {
        imageModel: model,
        model,
        size: "2048x2048",
        transparentBackground: "false",
        channels: [
            {
                id: "ch-seedream",
                scope: "system",
                enabled: true,
                name: "可美",
                baseUrl: "/api/ai/system/CHANNEL",
                apiFormat: "openai",
                concurrencyLimit: 0,
                models: [model],
                modelDetails: [{ model, protocol: "km-kemei-seedream", capability: "image", priceConfigured: true }],
                hasApiKey: true,
                hasSecretKey: false,
                createdAt: "",
                updatedAt: "",
            },
        ],
    } as unknown as AiConfig;
}

describe("ImageSettingsPanel with km-kemei-seedream", () => {
    test("Pro/Flash 只显示 1K / 1.5K / 2K，不显示 4K，且分辨率分段只有一处", () => {
        for (const model of ["doubao-seedream-5-0-pro-260628", "doubao-seedream-5-0-flash-260915"]) {
            const html = renderToStaticMarkup(<ImageSettingsPanel config={seedreamConfig(model)} onConfigChange={() => {}} theme={canvasThemes.dark} />);
            expect((html.match(/分辨率/g) || []).length).toBe(1);
            expect(html).toContain("1K");
            expect(html).toContain("1.5K");
            expect(html).toContain("2K");
            expect(html).not.toContain("4K");
        }
    });
});
