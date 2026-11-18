import { expect, test } from "bun:test";
import { resolve } from "node:path";

test("Studio verifies workspace before cache hydration and discards stale responses", () => {
    const result = Bun.spawnSync([process.execPath, "test", "./test/fixtures/studio-workspace-entry.harness.tsx"], { cwd: resolve(import.meta.dir, ".."), timeout: 30000 });
    expect(result.exitCode, new TextDecoder().decode(result.stderr)).toBe(0);
}, 35000);
