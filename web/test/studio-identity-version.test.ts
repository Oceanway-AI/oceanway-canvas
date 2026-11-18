import { expect, test } from "bun:test";
import { Window } from "happy-dom";
import { STUDIO_IDENTITY_VERSION, STUDIO_STORAGE_NAME } from "../src/lib/studio-identity";
import { scopedStorageKey, scopedLocalStorage, setActiveUserScope } from "../src/lib/user-scope";
import { apiClient, http } from "../src/services/api/request";

test("current writes carry the identity contract and old caches cannot hydrate", async () => {
    const originalWindow = globalThis.window;
    const originalAdapter = apiClient.defaults.adapter;
    const dom = new Window();
    Object.defineProperty(globalThis, "window", { configurable: true, writable: true, value: dom });
    try {
        const oldScope = "c0f9ff99-1f61-578c-a17d-f378d6c92b19";
        dom.localStorage.setItem("yingce:active-user-scope", oldScope);
        dom.localStorage.setItem(`draft:user:${oldScope}`, "legacy");
        dom.localStorage.setItem("draft:user:101", "unversioned");
        setActiveUserScope("101");
        expect(scopedLocalStorage.getItem("draft")).toBeNull();
        scopedLocalStorage.setItem("draft", "current");
        expect(scopedStorageKey("draft")).toBe(`${STUDIO_STORAGE_NAME}:draft:user:101`);
        expect(scopedLocalStorage.getItem("draft")).toBe("current");
        setActiveUserScope("202");
        expect(scopedLocalStorage.getItem("draft")).toBeNull();
        apiClient.defaults.adapter = async (config) => {
            expect(config.headers.get("X-Studio-Identity-Version")).toBe(STUDIO_IDENTITY_VERSION);
            expect(config.headers.get("X-Studio-Actor-ID")).toBe("202");
            expect(config.method).toBe("patch");
            expect(config.url).toBe("/canvas-projects/example/operations");
            expect(config.data).toBe('{"mutationId":"actor-switch"}');
            return { config, status: 200, statusText: "OK", headers: {}, data: { code: 0, data: {}, msg: "ok" } };
        };
        const pending = http.patch("/canvas-projects/example/operations", { mutationId: "actor-switch" });
        setActiveUserScope("303");
        await pending;
    } finally {
        setActiveUserScope(null);
        apiClient.defaults.adapter = originalAdapter;
        Object.defineProperty(globalThis, "window", { configurable: true, writable: true, value: originalWindow });
        await dom.happyDOM.abort();
    }
});
