import { expect, test } from "bun:test";
import { resolve } from "node:path";

test("登录、注册及找回密码入口只使用 Platform 登录，不恢复旧账号表单", () => {
    const result = Bun.spawnSync([process.execPath, "test", "./test/fixtures/platform-auth-entry.harness.tsx"], { cwd: resolve(import.meta.dir, ".."), timeout: 30000 });
    expect(result.exitCode, new TextDecoder().decode(result.stderr)).toBe(0);
}, 35000);
