import { expect, test } from "bun:test";
import { loadCreationRuntime } from "@/pages/create/creation-runtime-loader";

test("an open form reuses both the in-flight and the settled creation runtime", async () => {
    const first = loadCreationRuntime();
    const concurrent = loadCreationRuntime();
    const runtime = await first;
    expect(typeof runtime.skillRuntime.prepare).toBe("function");
    expect(concurrent).toBe(first);
    expect(loadCreationRuntime()).toBe(first);
});
