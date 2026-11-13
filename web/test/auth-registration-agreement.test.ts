import { describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";

const read = (path: string) => readFileSync(resolve(import.meta.dir, "../src", path), "utf8");

describe("注册服务协议", () => {
    test("公开认证设置暴露协议字段，管理接口可回传协议正文", () => {
        const auth = read("services/api/auth.ts");
        const wallet = read("services/api/wallet.ts");
        const panel = read("pages/admin/components/access-settings-panel.tsx");

        expect(auth).toContain("agreementTitle?: string;");
        expect(auth).toContain("agreementContent?: string;");
        expect(wallet).toContain("agreementTitle?: string");
        expect(wallet).toContain("agreementContent?: string");
        expect(panel).toContain("服务协议名称与条款");
        expect(panel).toContain("saveAgreement");
    });
});
