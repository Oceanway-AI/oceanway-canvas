import { describe, expect, test } from "bun:test";
import { taskResultMedia, taskResultMediaAccess } from "../src/pages/tasks/task-result-media";
import type { GenerationTask } from "../src/services/api/task-center";
import type { ResourceAccess } from "../src/services/api/resources";

const task = (result: unknown, extra: Partial<GenerationTask> = {}): GenerationTask => ({
    id: "task-1",
    provider: "platform",
    type: "canvas_image",
    status: "succeeded",
    prompt: "",
    attempts: 1,
    createdAt: "",
    updatedAt: "",
    resultJson: JSON.stringify(result),
    ...extra,
});
const access = (resourceId: string, url: string): ResourceAccess => ({ resourceId, url, requestedVariant: "preview", actualVariant: "preview", delivery: "cdn", issuedAt: "", refreshAt: "", revision: "v1" });

describe("task detail media uses adopted Platform resources", () => {
    test("reads the real successful result shape without requiring a URL extension", () => {
        const items = taskResultMedia(task({ mode: "image", images: [{ storageKey: "resource:material-1", dataUrl: "/api/resources/material-1/file", mimeType: "image/png", bytes: 1755769 }] }));
        expect(items).toEqual([{ key: "material-1", resourceId: "material-1", kind: "image" }]);
    });

    test("keeps multiple outputs and video resource identity, without adopting raw upstream URLs", () => {
        expect(taskResultMedia(task({ images: [{ storageKey: "resource:a" }, { storageKey: "resource:b" }, { storageKey: "resource:a" }, { dataUrl: "https://provider.invalid/raw.png" }] }))).toHaveLength(2);
        expect(taskResultMedia(task({ mode: "video", video: { storageKey: "resource:video-1", url: "https://provider.invalid/raw.mp4" } }, { type: "canvas_video" }))).toEqual([{ key: "video-1", resourceId: "video-1", kind: "video" }]);
        expect(taskResultMedia(task({ images: [{ storageKey: "resource:../other" }, { url: "https://provider.invalid/raw.png" }] }))).toEqual([]);
    });

    test("accepts only ready materialized outputs and deduplicates the structured result", () => {
        const output = { outputIndex: 0, mediaType: "image" as const, materializedResourceId: "asset-1" };
        expect(taskResultMedia(task(null, { resultState: "MATERIALIZING", outputs: [output] }))).toEqual([]);
        expect(taskResultMedia(task({ images: [{ storageKey: "resource:asset-1" }] }, { resultState: "READY", outputs: [output] }))).toHaveLength(1);
    });

    test("gets preview, original and playback from access authority with no guessed URL", async () => {
        const calls: unknown[][] = [];
        const read = async (...args: [string, "display", "preview" | "original" | "playback"]) => {
            calls.push(args);
            return access("asset-1", `https://authorized.invalid/${args[2]}`);
        };
        const item = { key: "asset-1", resourceId: "asset-1", kind: "image" as const };
        for (const variant of ["preview", "original", "playback"] as const) {
            const url = await taskResultMediaAccess(item, variant, (...args) => read(args[0], args[1], args[2] as "preview" | "original" | "playback"));
            expect(url).toBe(`https://authorized.invalid/${variant}`);
        }
        expect(calls).toEqual([
            ["resource:asset-1", "display", "preview"],
            ["resource:asset-1", "display", "original"],
            ["resource:asset-1", "display", "playback"],
        ]);
    });

    test("does not show a different resource or recover from an authorization failure using raw dataUrl", async () => {
        const [item] = taskResultMedia(task({ images: [{ storageKey: "resource:asset-1", dataUrl: "https://provider.invalid/raw.png" }] }));
        await expect(taskResultMediaAccess(item, "preview", async () => access("asset-2", "https://authorized.invalid/wrong"))).rejects.toThrow("素材访问身份不匹配");
        await expect(
            taskResultMediaAccess(item, "preview", async () => {
                throw new Error("Access denied");
            }),
        ).rejects.toThrow("Access denied");
    });
});
