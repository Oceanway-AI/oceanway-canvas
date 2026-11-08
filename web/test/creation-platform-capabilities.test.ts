import { expect, test } from "bun:test";
import { resolve } from "node:path";

test("创作首页支持 Platform 单模式/空目录与过期模型偏好", () => {
    const result = Bun.spawnSync([process.execPath, "test", "./test/fixtures/creation-platform-capabilities.harness.tsx"], { cwd: resolve(import.meta.dir, ".."), timeout: 30000 });
    expect(result.exitCode, new TextDecoder().decode(result.stderr)).toBe(0);
}, 35000);
