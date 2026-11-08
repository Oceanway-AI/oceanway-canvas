import { expect, test } from "bun:test";
import { resolve } from "node:path";

test("旧登录会话重开先完成 Platform 素材授权初始化，再开放工作区", () => {
    const result = Bun.spawnSync([process.execPath, "test", "./test/fixtures/platform-session-reopen.harness.tsx"], { cwd: resolve(import.meta.dir, ".."), timeout: 30000 });
    expect(result.exitCode, new TextDecoder().decode(result.stderr)).toBe(0);
}, 35000);
