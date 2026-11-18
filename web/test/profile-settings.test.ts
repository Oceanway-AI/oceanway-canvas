import { describe, expect, test } from "bun:test";
import { settingsPath } from "../src/lib/settings-navigation";
import { getUserProfile } from "../src/services/api/profile";
import { apiClient } from "../src/services/api/request";

describe("Platform profile and Studio preferences", () => {
    test("creation setup opens the Platform model selection", () => {
        expect(settingsPath()).toBe("/settings?section=profile");
        expect(settingsPath(undefined, true)).toBe("/settings?section=models&continue=1");
        expect(settingsPath("channels", true)).toBe("/settings?section=models&continue=1");
    });

    test("profile reads preserve request cancellation", async () => {
        const original = apiClient.request;
        const requests: Array<Record<string, unknown>> = [];
        apiClient.request = (async (config: Record<string, unknown>) => {
            requests.push(config);
            return { data: { code: 0, data: { provider: "platform", user: { id: "101" } }, msg: "ok" }, status: 200 };
        }) as typeof apiClient.request;
        try {
            const signal = new AbortController().signal;
            expect(await getUserProfile(signal)).toMatchObject({ provider: "platform", user: { id: "101" } });
            expect(requests).toHaveLength(1);
            expect(requests[0]).toMatchObject({ url: "/auth/profile", method: "get", signal });
        } finally {
            apiClient.request = original;
        }
    });
});
