import { expect, test } from "bun:test";
import { resolve } from "node:path";
import { apiClient } from "@/services/api/request";
import { decideAgentApproval, quoteAgentMedia } from "@/services/api/agent";

test("真实平台媒体审批卡", () => {
    const result = Bun.spawnSync([process.execPath, "test", "./test/fixtures/studio-agent-media-approval.harness.tsx"], { cwd: resolve(import.meta.dir, ".."), timeout: 30000 });
    expect(result.exitCode, new TextDecoder().decode(result.stderr)).toBe(0);
}, 35000);

test("报价和批准通过不同请求，批准只携带已展示报价身份", async () => {
    const original = apiClient.defaults.adapter;
    const requests: { url?: string; data: unknown }[] = [];
    apiClient.defaults.adapter = async (config) => {
        requests.push({ url: config.url, data: JSON.parse(config.data) });
        return { config, headers: {}, status: 200, statusText: "OK", data: { code: 0, data: { id: "exact-quote", walletMaxPoints: "0.123456789", priceVersion: "price" } } };
    };
    try {
        const quote = await quoteAgentMedia("run", "approval", undefined, "0.123456789");
        expect(quote.walletMaxPoints).toBe("0.123456789");
        await decideAgentApproval("run", "approval", "approve", undefined, undefined, undefined, quote.id);
        expect(requests).toEqual([
            { url: "/agent/runs/run/approvals/approval/quote", data: { walletMaxPoints: "0.123456789" } },
            { url: "/agent/runs/run/approvals/approval/decision", data: { decision: "approve", quoteId: "exact-quote" } },
        ]);
    } finally {
        apiClient.defaults.adapter = original;
    }
});
