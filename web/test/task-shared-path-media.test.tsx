import { describe, expect, test } from "bun:test";
import { renderToStaticMarkup } from "react-dom/server";
import { TaskGridCard } from "@/pages/tasks/task-grid-card";
import { TaskListRow } from "@/pages/tasks/task-list-row";
import { apiBaseURL } from "@/services/api/request";
import type { GenerationTask } from "@/services/api/task-center";
import { earlyPreviewURL } from "@/services/task-early-previews";
import { useConfigStore } from "@/stores/use-config-store";

const noop = () => {};
const baseTask: GenerationTask = {
    id: "existing-task",
    provider: "platform",
    type: "canvas_image",
    status: "succeeded",
    prompt: "",
    attempts: 1,
    createdAt: "2026-11-17T00:00:00Z",
    updatedAt: "2026-11-17T00:00:00Z",
};

function renderCards(task: GenerationTask) {
    const common = { task, effectiveConfig: useConfigStore.getState().config, actingId: "", onOpen: noop, onRetry: noop };
    return [renderToStaticMarkup(<TaskListRow {...common} canvasById={new Map()} projectNameById={new Map()} creditsEnabled={false} onPreview={noop} />), renderToStaticMarkup(<TaskGridCard {...common} />)];
}

describe("task media uses the configured API path", () => {
    test("list and grid resolve stored image and poster paths", () => {
        for (const task of [
            { ...baseTask, previewKind: "image" as const, previewUrl: "/api/resources/image-1/file" },
            { ...baseTask, previewKind: "video" as const, previewUrl: "/api/resources/video-1/file", previewPosterUrl: "/api/resources/image-1/file" },
        ]) {
            for (const markup of renderCards(task)) expect(markup).toContain(`src="${apiBaseURL}/resources/image-1/file"`);
        }
    });

    test("verified early images use the same API path in list and grid", () => {
        const task = { ...baseTask, status: "running" as const, earlyPreviews: [{ index: 0, kind: "image" as const }] };
        const expected = `${apiBaseURL}/tasks/existing-task/previews/0`;
        expect(earlyPreviewURL({ taskId: task.id, index: 0, kind: "image" })).toBe(expected);
        for (const markup of renderCards(task)) expect(markup).toContain(`src="${expected}"`);
        expect(earlyPreviewURL({ taskId: "../other", index: 0, kind: "image" })).toBe("");
    });

    test("absolute CDN addresses keep their existing location", () => {
        for (const markup of renderCards({ ...baseTask, previewKind: "image", previewUrl: "https://cdn.example.test/image.webp" })) {
            expect(markup).toContain('src="https://cdn.example.test/image.webp"');
        }
    });
});
