import { expect, test } from "bun:test";

import { defaultFeatureAvailability } from "../src/stores/use-user-store";
import { planWorkbenchBatch, workbenchBatchLimit } from "../src/pages/workbench/workbench-batches";

test("no account cap keeps the batch size and per-task output constraints independent", () => {
    expect(workbenchBatchLimit("image", 1, 0)).toBe(12);
    expect(workbenchBatchLimit("video", 1, 0)).toBe(8);
    expect(planWorkbenchBatch("image", 5, 2)).toEqual([2, 2, 1]);
    expect(workbenchBatchLimit("video", 1, 3)).toBe(3);
});

test("new workbench entries stay closed until the backend explicitly opens them", () => {
    expect(defaultFeatureAvailability.imageWorkbenchEnabled).toBe(false);
    expect(defaultFeatureAvailability.videoWorkbenchEnabled).toBe(false);
});
