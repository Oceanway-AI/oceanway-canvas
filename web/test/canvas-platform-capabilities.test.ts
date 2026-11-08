import { expect, test } from "bun:test";
import { resolve } from "node:path";

test("画布图片/视频节点支持平台单能力目录、空能力和旧节点模型", () => {
    const result = Bun.spawnSync([process.execPath, "test", "./test/fixtures/canvas-platform-capabilities.harness.tsx"], { cwd: resolve(import.meta.dir, ".."), timeout: 30000 });
    expect(result.exitCode, new TextDecoder().decode(result.stderr)).toBe(0);
}, 35000);
