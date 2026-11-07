import { expect, test } from "bun:test";
import { apiClient } from "../src/services/api/request";
import { getAuthSession, invalidateAuthSessionCache } from "../src/services/api/auth";

test("a session response started before logout cannot replace the new identity", async () => {
    const original = apiClient.request;
    const responses: Array<(value: unknown) => void> = [];
    apiClient.request = (() => new Promise((resolve) => responses.push(resolve))) as typeof apiClient.request;
    try {
        invalidateAuthSessionCache();
        const oldRequest = getAuthSession().catch((error) => error);
        invalidateAuthSessionCache();
        const currentRequest = getAuthSession();
        responses[0]({ status: 200, data: { code: 0, data: { user: { id: "old-user" } }, msg: "ok" } });
        expect(await oldRequest).toBeInstanceOf(Error);
        // Finishing the old request must not clear the current in-flight request.
        expect(getAuthSession()).toBe(currentRequest);
        responses[1]({ status: 200, data: { code: 0, data: { user: null }, msg: "ok" } });
        expect((await currentRequest).user).toBeNull();
        expect((await getAuthSession()).user).toBeNull();
        expect(responses).toHaveLength(2);
    } finally {
        apiClient.request = original;
        invalidateAuthSessionCache();
    }
});
