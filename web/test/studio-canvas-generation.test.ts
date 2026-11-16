import { expect, test } from "bun:test";
import { projectStudioCanvasOutput } from "@/lib/canvas/studio-generation-outputs";
import { CanvasNodeType, type CanvasNodeData } from "@/types/canvas";
import type { GenerationTask } from "@/services/api/task-center";

test("three ordinals adopt independently, preserving edits and tombstones after replacement or deletion", () => {
    const task = { id: "task-A", platformGenerationRequest: "request-A", platformRevision: 1, projectId: "project", status: "running", type: "canvas_image" } as GenerationTask;
    const root = { id: "root", title: "edit", type: CanvasNodeType.Image, position: { x: 9, y: 20 }, width: 193, height: 181, metadata: { taskId: "task-A" } } as CanvasNodeData;
    const slot = (index: number, resourceId?: string) => ({
        index,
        kind: "image" as const,
        contentState: "available",
        previewState: "pending",
        materialState: resourceId ? "saved" : "saving",
        resourceId,
        media: resourceId ? { storageKey: "resource:" + resourceId, dataUrl: "/api/resources/" + resourceId + "/file", width: 33, height: 19 } : undefined,
    });
    let nodes = [root];
    for (const entry of [slot(0, "r0"), slot(1), slot(2, "r2")]) nodes = projectStudioCanvasOutput(nodes, "root", task, entry).nodes;
    expect(nodes).toHaveLength(3);
    expect(nodes.map((n) => [n.metadata?.platformOutputIndex, n.metadata?.storageKey])).toEqual([
        [0, "resource:r0"],
        [1, undefined],
        [2, "resource:r2"],
    ]);
    nodes = nodes.map((n, i) => (i === 1 ? { ...n, title: "moved", position: { x: 143, y: 201 }, width: 211, height: 79 } : n));
    nodes = projectStudioCanvasOutput(nodes, "root", { ...task, platformRevision: 2 }, slot(1, "r1")).nodes;
    expect(nodes[1]).toMatchObject({ title: "moved", position: { x: 143, y: 201 }, width: 211, height: 79, metadata: { storageKey: "resource:r1" } });
    expect(projectStudioCanvasOutput(nodes, "root", task, slot(1, "r1")).nodes).toEqual(nodes);
    const deleted = nodes.filter((n) => n.id !== nodes[1].id);
    expect(projectStudioCanvasOutput(deleted, "root", task, slot(1, "r1")).nodes).toEqual(deleted);
    const rebound = nodes.map((n) => (n.id === "root" ? { ...n, metadata: { ...n.metadata, taskId: "task-B" } } : n));
    expect(projectStudioCanvasOutput(rebound, "root", task, slot(0, "r0")).nodes).toEqual(rebound);
});
