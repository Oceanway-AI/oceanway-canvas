import { expect, test } from "bun:test";
import type { GenerationTask } from "@/services/api/task-center";
import { createStudioGenerationReads } from "@/services/studio-generation-reads";

const task = (revision: number, status: GenerationTask["status"] = "running"): GenerationTask => ({ id: "task", platformGenerationRequest: "original", platformRevision: revision, retryAfterMs: 2000, status }) as GenerationTask;

test("receipt, detail and canvas use one read per interval and drop stale or cross-scope responses", async () => {
    let time = 0,
        scope = "account/workspace",
        reads = 0;
    let release!: (v: GenerationTask) => void;
    const reader = createStudioGenerationReads(
        () => scope,
        async () => {
            reads++;
            return new Promise<GenerationTask>((r) => {
                release = r;
            });
        },
        () => time,
    );
    reader.remember(task(1));
    expect((await reader.read("task")).platformRevision).toBe(1);
    expect(reads).toBe(0);
    time = 2001;
    const a = new AbortController();
    const first = reader.read("task", a.signal);
    const second = reader.read("task");
    a.abort();
    release(task(2));
    await expect(first).rejects.toThrow();
    expect((await second).platformRevision).toBe(2);
    expect(reads).toBe(1);
    reader.remember(task(1));
    expect((await reader.read("task")).platformRevision).toBe(2);
    time += 2200;
    const late = reader.read("task");
    scope = "other/space";
    release(task(3));
    await expect(late).rejects.toThrow();
    reader.remember(task(9, "succeeded"));
    time += 10000;
    expect((await reader.read("task")).platformRevision).toBe(9);
    expect(reads).toBe(2);
});
