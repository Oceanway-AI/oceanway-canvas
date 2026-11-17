import { expect, test } from "bun:test";
import { studioTaskUsageURL } from "@/services/api/studio-account";
import { resolve } from "node:path";

test("Studio binds its shared account bar to the signed-in account and exact wallet amount", () => {
    const result = Bun.spawnSync([process.execPath, "test", "./test/fixtures/studio-account.harness.tsx"], { cwd: resolve(import.meta.dir, ".."), timeout: 30000 });
    expect(result.exitCode, new TextDecoder().decode(result.stderr)).toBe(0);
}, 35000);

const usageUrl = "https://console.example/console/records";
test("Console filters the original grant's task reference and preserves product ownership", () => {
    const direct = new URL(studioTaskUsageURL(usageUrl, { id: "task & 1" })!);
    expect(Object.fromEntries(direct.searchParams)).toEqual({ source: "platform", product: "studio", business_task_id: "task & 1" });
    const agent = new URL(studioTaskUsageURL(usageUrl, { id: "media", agentRunId: "original run" })!);
    expect(agent.searchParams.get("business_task_id")).toBe("original run");
    expect(studioTaskUsageURL(undefined, { id: "wrong" })).toBeUndefined();
});

test("Studio homepage links retain the deployed application root", () => {
    const result = Bun.spawnSync([process.execPath, "--define", 'import.meta.env.BASE_URL="/studio/"', "test", "./test/fixtures/studio-account.harness.tsx"], {
        cwd: resolve(import.meta.dir, ".."),
        timeout: 30000,
        env: { ...process.env, STUDIO_NAVIGATION_TEST_PATH: "/" },
    });
    expect(result.exitCode, new TextDecoder().decode(result.stderr)).toBe(0);
}, 35000);
