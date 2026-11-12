import { afterAll, afterEach, beforeEach, expect, test } from "bun:test";
import { resolve } from "node:path";

// Session hydration owns browser-wide stores; isolate the real stores from
// unrelated test files while exercising the production HTTP client.
if (process.env.MODEL_CATALOG_REFRESH_CHILD !== "1") {
    test("model catalog refresh and session isolation", () => {
        const result = Bun.spawnSync([process.execPath, "test", "./test/model-catalog-refresh.test.ts"], {
            cwd: resolve(import.meta.dir, ".."),
            env: { ...process.env, MODEL_CATALOG_REFRESH_CHILD: "1" },
            timeout: 30000,
        });
        expect(result.exitCode, new TextDecoder().decode(result.stderr)).toBe(0);
    }, 35000);
} else {
    const { Window } = await import("happy-dom");
    const dom = new Window({ url: "https://studio.example.test/" });
    Object.defineProperty(globalThis, "window", { configurable: true, value: dom });
    Object.defineProperty(globalThis, "document", { configurable: true, value: dom.document });
    Object.defineProperty(globalThis, "navigator", { configurable: true, value: dom.navigator });
    const { default: localforage } = await import("localforage");
    const storage = new Map<string, unknown>();
    localforage.getItem = async <T>(key: string) => (storage.get(key) as T) ?? null;
    localforage.setItem = async <T>(key: string, value: T) => {
        storage.set(key, value);
        return value;
    };
    localforage.removeItem = async (key: string) => {
        storage.delete(key);
    };
    const { apiClient } = await import("@/services/api/request");
    const { refreshSystemChannels, applyUserSession } = await import("@/lib/user-session");
    const { useConfigStore, createModelChannel, normalizeConfigSnapshot, CONFIG_STORE_KEY } = await import("@/stores/use-config-store");
    const { useUserStore } = await import("@/stores/use-user-store");
    const { useModelCatalogStore } = await import("@/stores/use-model-catalog-store");
    const { setActiveUserScope, scopedLocalStorage } = await import("@/lib/user-scope");
    const { resetRemoteUserDataSync } = await import("@/services/user-data-sync");
    const { appQueryClient } = await import("@/lib/query-client");
    const user = (id: string) => ({ id, username: id, displayName: id, role: "user" as const, status: "active" as const, createdAt: "2026-11-13T00:00:00Z", updatedAt: "2026-11-13T00:00:00Z" });
    const custom = createModelChannel({ id: "custom", name: "Custom", scope: "user", baseUrl: "https://example.test", apiKey: "fixture", models: ["custom-text"] });
    const stale = createModelChannel({ id: "platform", name: "Platform", scope: "system", models: ["stale-model"] });
    const modelNames = () => useConfigStore.getState().config.channels.flatMap((channel) => channel.models);
    const catalog = (name: string) => ({
        source: "platform",
        channels: [{ id: "platform", name: "Platform", displayName: "Platform", models: [{ id: name, modelKey: name, displayName: name, capability: "text", available: true, priceTiers: [], pricingMode: "platform", priceLabel: "", icon: "" }] }],
    });
    function deferred<T>() {
        let resolve!: (value: T) => void;
        let reject!: (error: unknown) => void;
        const promise = new Promise<T>((yes, no) => {
            resolve = yes;
            reject = no;
        });
        return { promise, resolve, reject };
    }
    let loadCatalog: () => Promise<unknown>;
    let requests = 0;
    beforeEach(() => {
        resetRemoteUserDataSync();
        storage.clear();
        dom.localStorage.clear();
        setActiveUserScope("account-a");
        useUserStore.setState({ user: user("account-a"), hydrated: true });
        useModelCatalogStore.setState({ status: "idle", error: null });
        useConfigStore.getState().replaceConfig(normalizeConfigSnapshot({ config: { channels: [stale, custom] } }).config);
        requests = 0;
        loadCatalog = async () => catalog("fresh-model");
        apiClient.defaults.adapter = async (config) => {
            let data: unknown;
            if (config.url === "/model-catalog") {
                requests++;
                data = await loadCatalog();
            } else if (config.url === "/user-data/snapshot") data = { assets: [], projects: [] };
            else throw new Error(`Unexpected fixture request ${config.url}`);
            return { config, status: 200, statusText: "OK", headers: {}, data: { code: 0, data } };
        };
    });
    afterEach(() => {
        resetRemoteUserDataSync();
        appQueryClient.clear();
    });
    afterAll(() => dom.happyDOM.abort());

    test("refresh failure removes stale system models and preserves custom channels", async () => {
        loadCatalog = async () => {
            throw new Error("fixture unavailable");
        };
        await expect(refreshSystemChannels()).rejects.toThrow("fixture unavailable");
        expect(modelNames()).toEqual(["custom-text"]);
        expect(useConfigStore.getState().config.channels[0]).toMatchObject(custom);
        expect(useModelCatalogStore.getState().status).toBe("error");
        expect(useModelCatalogStore.getState().error).toContain("重试");
        loadCatalog = async () => catalog("fresh-model");
        await refreshSystemChannels();
        expect(useModelCatalogStore.getState()).toEqual({ status: "ready", error: null });
        expect(modelNames()).toEqual(["fresh-model", "custom-text"]);
    });

    test("same-session concurrent refresh uses one catalog request", async () => {
        const response = deferred<unknown>();
        loadCatalog = () => response.promise;
        const first = refreshSystemChannels();
        const second = refreshSystemChannels();
        expect(useModelCatalogStore.getState().status).toBe("loading");
        expect(modelNames()).toEqual(["custom-text"]);
        await Promise.resolve();
        response.resolve(catalog("fresh-model"));
        await Promise.all([first, second]);
        expect(requests).toBe(1);
        expect(modelNames()).toEqual(["fresh-model", "custom-text"]);
        expect(useModelCatalogStore.getState().status).toBe("ready");
    });

    test("late response from an old identity cannot replace the current catalog", async () => {
        const old = deferred<unknown>();
        loadCatalog = () => old.promise;
        const pending = refreshSystemChannels();
        await Promise.resolve();
        useUserStore.setState({ user: user("account-b") });
        loadCatalog = async () => catalog("account-b-model");
        await refreshSystemChannels();
        old.resolve(catalog("account-a-model"));
        await pending;
        expect(modelNames()).toEqual(["account-b-model", "custom-text"]);
        expect(useModelCatalogStore.getState().status).toBe("ready");
    });

    test("hydration failure invalidates persisted system models", async () => {
        scopedLocalStorage.setItem(CONFIG_STORE_KEY, JSON.stringify({ state: { config: useConfigStore.getState().config }, version: 0 }));
        loadCatalog = async () => {
            throw new Error("fixture unavailable");
        };
        await applyUserSession({ provider: "platform", user: user("account-a") });
        await new Promise((resolve) => setTimeout(resolve, 20));
        expect(modelNames()).toEqual(["custom-text"]);
        expect(useModelCatalogStore.getState().status).toBe("error");
    });

    test("new session for the same identity ignores a late old failure", async () => {
        const old = deferred<unknown>();
        loadCatalog = () => old.promise;
        const pending = refreshSystemChannels();
        await new Promise((resolve) => setTimeout(resolve, 0));
        loadCatalog = async () => catalog("new-session-model");
        await applyUserSession({ provider: "platform", user: user("account-a") });
        await refreshSystemChannels();
        old.reject(new Error("old session failure"));
        await pending;
        expect(useModelCatalogStore.getState()).toEqual({ status: "ready", error: null });
        expect(modelNames()).toEqual(["new-session-model", "custom-text"]);
    });

    test("malformed successful HTTP response is a catalog error", async () => {
        loadCatalog = async () => ({ source: "platform", channels: null });
        await expect(refreshSystemChannels()).rejects.toThrow();
        expect(modelNames()).toEqual(["custom-text"]);
        expect(useModelCatalogStore.getState().status).toBe("error");
    });

    test("logout invalidates pending catalog and leaves idle state", async () => {
        const old = deferred<unknown>();
        loadCatalog = () => old.promise;
        const pending = refreshSystemChannels();
        await new Promise((resolve) => setTimeout(resolve, 0));
        await applyUserSession({ provider: "platform", user: null });
        old.resolve(catalog("logged-out-model"));
        await pending;
        expect(useModelCatalogStore.getState()).toEqual({ status: "idle", error: null });
        expect(modelNames()).not.toContain("logged-out-model");
        expect(useConfigStore.getState().config.channels.every((channel) => channel.scope !== "system")).toBe(true);
    });
}
