import { describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { getWorkspaceAntThemeConfig } from "../src/lib/app-theme";
describe("asset library header buttons", () => {
    test("user pages share control geometry without a light-only outline override", () => {
        const css = readFileSync(resolve(import.meta.dir, "../src/styles/globals.css"), "utf8");
        const studio = readFileSync(resolve(import.meta.dir, "../src/styles/oceanway.css"), "utf8");
        const theme = getWorkspaceAntThemeConfig();
        // 用户工作台唯一按钮尺度：40px 常规 / 32px 紧凑，按钮与输入控件共用 14px 圆角。
        expect(theme.token?.controlHeight).toBe(40);
        expect(theme.token?.controlHeightSM).toBe(32);
        expect(theme.components?.Button?.borderRadius).toBe(theme.token?.borderRadius);
        expect(theme.components?.Input?.borderRadius).toBe(theme.token?.borderRadius);
        expect(theme.components?.Select?.borderRadius).toBe(theme.token?.borderRadius);
        expect(studio).toContain("--user-control-height: 40px;");
        expect(studio).toContain("--user-radius-button: 14px;");
        // 次要按钮是带 1px 边缘的实体键，明暗主题共用 button-tokens.css 的同一组组件变量，而不是亮色专用覆盖。
        expect(theme.components?.Button?.defaultBg).toBe("var(--button-secondary-bg)");
        expect(theme.components?.Button?.defaultBorderColor).toBe("var(--button-secondary-border)");
        expect(theme.components?.Button?.textTextColor).toBe("var(--button-ghost-fg)");
        expect(css).not.toContain("html:not(.dark) .library-page .app-page-header .ant-btn");
        expect(css).not.toContain("html .assets-library-page .app-page-header .assets-header-action-buttons .ant-btn");
    });
});
