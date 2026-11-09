import { expect, test } from "bun:test";
import { creationResultAssetIds, canvasAssetHandoffPayloads } from "@/lib/canvas/canvas-asset-handoff";
import type { Asset } from "@/stores/use-asset-store";

const adopted = {
    id: "platform-r1",
    kind: "image",
    title: "用户已编辑",
    tags: [],
    metadata: { source: "platform-library" },
    data: { storageKey: "resource:r1", dataUrl: "/api/resources/r1/file", width: 640, height: 427, bytes: 128, mimeType: "image/png" },
} as unknown as Asset;

test("explicit native adoption matches a generated result by fixed Resource without rewriting metadata", () => {
    expect(creationResultAssetIds([adopted], { messageId: "m1", taskIds: ["t1"], resultUrls: [], resultStorageKeys: ["resource:r1"] })).toEqual(["platform-r1"]);
});

test("a fixed Resource result never falls back to another output or an empty URL", () => {
    const unrelated = { ...adopted, metadata: { source: "generation-task", taskId: "t1" }, coverUrl: "" };
    expect(creationResultAssetIds([unrelated], { messageId: "m1", taskIds: ["t1"], resultUrls: [], resultStorageKeys: ["resource:r2"] })).toEqual([]);
});

test("handoff retains the original image dimensions and stable identity", () => {
    expect(canvasAssetHandoffPayloads([adopted], [adopted.id]).payloads[0]).toMatchObject({ assetId: "platform-r1", storageKey: "resource:r1", width: 640, height: 427, bytes: 128, mimeType: "image/png" });
});
