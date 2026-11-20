import { expect, test } from "bun:test";
import { resolve } from "node:path";
import { apiClient } from "@/services/api/request";
import { decideAgentApproval, prepareAgentMediaApproval } from "@/services/api/agent";

test("真实平台媒体审批卡", () => {
    const result = Bun.spawnSync([process.execPath, "test", "./test/fixtures/studio-agent-media-approval.harness.tsx"], { cwd: resolve(import.meta.dir, ".."), timeout: 30000 });
    expect(result.exitCode, new TextDecoder().decode(result.stderr)).toBe(0);
}, 35000);

test("生成使用后台取得的按量报价身份，旧草稿保留已绑定的上限", async () => {
    const original = apiClient.defaults.adapter;
    const requests: { url?: string; data: unknown }[] = [];
    apiClient.defaults.adapter = async (config) => {
        requests.push({ url: config.url, data: JSON.parse(config.data) });
        return { config, headers: {}, status: 200, statusText: "OK", data: { code: 0, data: { id: "exact-quote", billingMode: "actual_usage" } } };
    };
    try {
        const legacy = { id: "old", walletMaxPoints: "1.00", priceVersion: "old", expiresAt: "2099-01-01T00:00:00Z", parameters: {} };
        expect(await prepareAgentMediaApproval("run", "approval", legacy)).toBe("old");
        await expect(prepareAgentMediaApproval("run", "approval", { ...legacy, expiresAt: "2000-01-01T00:00:00Z" })).rejects.toThrow("旧草稿已过期");
        const quoteId = await prepareAgentMediaApproval("run", "approval");
        await decideAgentApproval("run", "approval", "approve", undefined, undefined, undefined, quoteId);
        expect(requests).toEqual([
            { url: "/agent/runs/run/approvals/approval/quote", data: { billingMode: "actual_usage" } },
            { url: "/agent/runs/run/approvals/approval/decision", data: { decision: "approve", quoteId: "exact-quote" } },
        ]);
    } finally {
        apiClient.defaults.adapter = original;
    }
});
