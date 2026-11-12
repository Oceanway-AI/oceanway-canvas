import { expect, test } from "bun:test";
import { resolve } from "node:path";

test("native image settings preserve exact dimensions across interactions and canvas recovery", () => {
    const result = Bun.spawnSync([process.execPath, "test", "./test/fixtures/platform-native-image-size-ui.harness.tsx"], { cwd: resolve(import.meta.dir, ".."), timeout: 30000 });
    const output = new TextDecoder().decode(result.stdout) + new TextDecoder().decode(result.stderr);
    expect(result.exitCode, output).toBe(0);
}, 35000);
