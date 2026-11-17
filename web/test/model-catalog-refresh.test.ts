import { afterAll, afterEach, beforeEach, expect, spyOn, test } from "bun:test";
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
    for (const [key, value] of Object.entries({
        HTMLElement: dom.HTMLElement,
        SVGElement: dom.SVGElement,
        Element: dom.Element,
        Node: dom.Node,
        Event: dom.Event,
        CustomEvent: dom.CustomEvent,
        ShadowRoot: dom.ShadowRoot,
        MutationObserver: dom.MutationObserver,
        ResizeObserver: dom.ResizeObserver,
        getComputedStyle: dom.getComputedStyle.bind(dom),
        IS_REACT_ACT_ENVIRONMENT: true,
    }))
        Object.defineProperty(globalThis, key, { configurable: true, writable: true, value });
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
    const { refreshSystemChannels, applyUserSession, ensurePlatformModelDetail } = await import("@/lib/user-session");
    const { useConfigStore, createModelChannel, normalizeConfigSnapshot, CONFIG_STORE_KEY } = await import("@/stores/use-config-store");
    const { useUserStore } = await import("@/stores/use-user-store");
    const { useModelCatalogStore } = await import("@/stores/use-model-catalog-store");
    const { setActiveUserScope, scopedLocalStorage, scopedStorageKey } = await import("@/lib/user-scope");
    const { resetRemoteUserDataSync } = await import("@/services/user-data-sync");
    const { appQueryClient } = await import("@/lib/query-client");
    const user = (id: string) => ({ id, username: id, displayName: id, role: "user" as const, status: "active" as const, createdAt: "2026-11-13T00:00:00Z", updatedAt: "2026-11-13T00:00:00Z" });
    const custom = createModelChannel({ id: "custom", name: "Custom", scope: "user", baseUrl: "https://example.test", apiKey: "fixture", models: ["custom-text"] });
    const stale = createModelChannel({ id: "platform", name: "Platform", scope: "system", models: ["stale-model"] });
    const modelNames = () => useConfigStore.getState().config.channels.flatMap((channel) => channel.models);
    const catalog = (name: string) => ({
        source: "platform",
        catalogRevision: "catalog-v1",
        hasMore: false,
        channels: [{ id: "platform", name: "Platform", displayName: "Platform", models: [{ id: name, modelKey: name, displayName: name, capability: "text", available: true, priceTiers: [], pricingMode: "platform", priceLabel: "", icon: "" }] }],
    });
    const multipleModels = (...names: string[]) => ({ ...catalog(names[0]), channels: [{ ...catalog(names[0]).channels[0], models: names.map((name) => catalog(name).channels[0].models[0]) }] });
    const chooseModel = (value: string) => {
        useConfigStore.getState().updateConfig("model", value);
        useConfigStore.getState().updateConfig("textModel", value);
    };
    function deferred<T>() {
        let resolve!: (value: T) => void;
        let reject!: (error: unknown) => void;
        const promise = new Promise<T>((yes, no) => {
            resolve = yes;
            reject = no;
        });
        return { promise, resolve, reject };
    }
    let loadCatalog: (cursor?: string) => Promise<unknown>;
    let loadDetail: (name: string) => Promise<unknown>;
    let requests = 0;
    beforeEach(() => {
        resetRemoteUserDataSync();
        storage.clear();
        dom.localStorage.clear();
        setActiveUserScope("account-a");
        useUserStore.setState({ user: user("account-a"), hydrated: true, platformIdentity: true });
        useModelCatalogStore.setState({ status: "idle", error: null });
        useConfigStore.setState({ pendingModelSelections: {} });
        useConfigStore.getState().replaceConfig(normalizeConfigSnapshot({ config: { channels: [stale, custom] } }).config);
        requests = 0;
        loadCatalog = async () => catalog("fresh-model");
        loadDetail = async () => {
            throw new Error("Unexpected detail request");
        };
        apiClient.defaults.adapter = async (config) => {
            let data: unknown;
            if (config.url === "/model-catalog/summaries") {
                requests++;
                data = await loadCatalog(config.params?.cursor);
            } else if (config.url === "/model-catalog/detail") {
                data = await loadDetail(config.params?.model_name);
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

    const mediaCatalog = (name: string, state: "summary" | "detail" = "summary") => {
        const response = catalog(name);
        return {
            ...response,
            channels: [
                {
                    ...response.channels[0],
                    models: [
                        {
                            ...response.channels[0].models[0],
                            capability: "video",
                            platform: {
                                model_name: name,
                                model_type: "video",
                                state,
                                price: { status: "available", version: "p1", rates: {} },
                                price_version: "p1",
                                capability_revision: "c1",
                                connection_revision: 1,
                                ...(state === "detail" ? { media_generation_profile: { variants: [], marker: "exact-detail" } } : {}),
                            },
                        },
                    ],
                },
            ],
        };
    };

    test("summary pages appear progressively and retain a later selected model", async () => {
        loadCatalog = async () => multipleModels("first-model", "chosen-model");
        await refreshSystemChannels();
        chooseModel("platform::chosen-model");
        const second = deferred<unknown>();
        loadCatalog = async (cursor) => (cursor ? second.promise : { ...catalog("first-model"), hasMore: true, nextCursor: "page-2" });
        const pending = refreshSystemChannels();
        await Bun.sleep(10);
        expect(modelNames()).toContain("first-model");
        expect(useConfigStore.getState().pendingModelSelections.textModel).toBe("platform::chosen-model");
        second.resolve(catalog("chosen-model"));
        await pending;
        expect(useConfigStore.getState().config.textModel).toBe("platform::chosen-model");
        expect(requests).toBe(3);
    });

    test("model detail requests coalesce and stay out of browser persistence", async () => {
        loadCatalog = async () => mediaCatalog("video-model");
        await refreshSystemChannels();
        let details = 0;
        loadDetail = async (name) => {
            details++;
            return { model: mediaCatalog(name, "detail").channels[0].models[0] };
        };
        await Promise.all([ensurePlatformModelDetail("platform::video-model"), ensurePlatformModelDetail("platform::video-model")]);
        await ensurePlatformModelDetail("platform::video-model");
        expect(details).toBe(1);
        expect(useConfigStore.getState().config.channels[0].modelCosts?.[0].platform?.state).toBe("detail");
        expect(scopedLocalStorage.getItem(CONFIG_STORE_KEY)).not.toContain("exact-detail");
    });

    test("the mounted picker loads its saved selection and clears the loading state", async () => {
        const { act, createElement } = await import("react");
        const { createRoot } = await import("react-dom/client");
        const { ModelPicker } = await import("@/components/model-picker");
        loadCatalog = async () => mediaCatalog("video-model");
        await refreshSystemChannels();
        const response = deferred<unknown>();
        let reads = 0;
        loadDetail = () => {
            reads++;
            return response.promise;
        };
        function Picker() {
            const config = useConfigStore((state) => state.config);
            return createElement(ModelPicker, { config, value: "platform::video-model", capability: "video", onChange: () => {} });
        }
        const host = document.createElement("div");
        document.body.append(host);
        const root = createRoot(host);
        try {
            await act(async () => root.render(createElement(Picker)));
            expect(host.textContent).toContain("正在加载模型参数");
            expect(reads).toBe(1);
            await act(async () => {
                response.resolve({ model: mediaCatalog("video-model", "detail").channels[0].models[0] });
                await response.promise;
            });
            expect(host.textContent).not.toContain("正在加载模型参数");
            expect(host.querySelector('[role="alert"]')).toBeNull();
            await act(async () => root.render(createElement(Picker)));
            expect(reads).toBe(1);
        } finally {
            await act(async () => root.unmount());
            host.remove();
        }
    });

    test("the picker rechecks availability after detail arrives before changing selection", async () => {
        const { act, createElement } = await import("react");
        const { createRoot } = await import("react-dom/client");
        const { ModelPicker } = await import("@/components/model-picker");
        loadCatalog = async () => mediaCatalog("video-model");
        await refreshSystemChannels();
        const response = deferred<unknown>();
        loadDetail = () => response.promise;
        const selected: string[] = [];
        function Picker() {
            const config = useConfigStore((state) => state.config);
            const loaded = config.channels[0]?.modelCosts?.[0].platform?.state === "detail";
            return createElement(ModelPicker, {
                config,
                capability: "video",
                onChange: (model: string) => selected.push(model),
                modelDisabledReason: () => (loaded ? "完整参数不支持当前输入" : ""),
            });
        }
        const host = document.createElement("div");
        document.body.append(host);
        const root = createRoot(host);
        try {
            await act(async () => root.render(createElement(Picker)));
            await act(async () => host.querySelector<HTMLButtonElement>("button")!.click());
            const option = document.querySelector<HTMLButtonElement>("[data-model-picker-item]");
            expect(option).not.toBeNull();
            await act(async () => option!.click());
            expect(selected).toEqual([]);
            await act(async () => {
                response.resolve({ model: mediaCatalog("video-model", "detail").channels[0].models[0] });
                await response.promise;
            });
            expect(selected).toEqual([]);
            expect(document.querySelector('[role="alert"]')?.textContent).toContain("完整参数不支持当前输入");
        } finally {
            await act(async () => root.unmount());
            host.remove();
        }
    });

    test("late detail cannot restore a failed or refreshed catalog", async () => {
        loadCatalog = async () => mediaCatalog("video-model");
        await refreshSystemChannels();
        const detail = deferred<unknown>();
        loadDetail = () => detail.promise;
        const pending = ensurePlatformModelDetail("platform::video-model");
        loadCatalog = async () => {
            throw new Error("unavailable");
        };
        await expect(refreshSystemChannels()).rejects.toThrow();
        detail.resolve({ model: mediaCatalog("video-model", "detail").channels[0].models[0] });
        await expect(pending).rejects.toThrow();
        expect(modelNames()).not.toContain("video-model");
    });

    test("mixed revisions and repeated cursors reject partial permissions", async () => {
        loadCatalog = async (cursor) => (cursor ? { ...catalog("second"), catalogRevision: "changed" } : { ...catalog("first"), hasMore: true, nextCursor: "page-2" });
        await expect(refreshSystemChannels()).rejects.toThrow("模型目录已更新");
        expect(modelNames()).toEqual(["custom-text"]);
        loadCatalog = async (cursor) => ({ ...catalog(cursor ? "second" : "first"), hasMore: true, nextCursor: "same" });
        await expect(refreshSystemChannels()).rejects.toThrow("模型目录分页响应无效");
        expect(modelNames()).toEqual(["custom-text"]);
    });

    test("a catalog larger than the browser quota remains in memory and never fills localStorage", async () => {
        const largeProfile = { pattern: "x".repeat(3 * 1024 * 1024) };
        const response = catalog("large-model");
        const model = { ...response.channels[0].models[0], platform: { model_name: "large-model", media_generation_profile: largeProfile } };
        loadCatalog = async () => ({ ...response, channels: [{ ...response.channels[0], models: [model] }] });
        const realSetItem = scopedLocalStorage.setItem;
        let maximumConfigBytes = 0;
        const quota = spyOn(scopedLocalStorage, "setItem").mockImplementation((key, value) => {
            const storageKey = scopedStorageKey(key);
            let bytes = (storageKey.length + value.length) * 2;
            for (let index = 0; index < dom.localStorage.length; index++) {
                const storedKey = dom.localStorage.key(index)!;
                if (storedKey !== storageKey) bytes += (storedKey.length + dom.localStorage.getItem(storedKey)!.length) * 2;
            }
            if (bytes > 5 * 1024 * 1024) throw new DOMException("Storage quota exceeded", "QuotaExceededError");
            if (key.startsWith(CONFIG_STORE_KEY)) maximumConfigBytes = Math.max(maximumConfigBytes, value.length * 2);
            realSetItem(key, value);
        });
        try {
            await expect(refreshSystemChannels()).resolves.toBeUndefined();
            expect(useModelCatalogStore.getState().status).toBe("ready");
            expect(useConfigStore.getState().config.channels[0].modelCosts?.[0].platform?.media_generation_profile).toEqual(largeProfile);
            chooseModel("platform::large-model");
            useConfigStore.getState().updateConfig("quality", "high");
            const stored = JSON.parse(scopedLocalStorage.getItem(CONFIG_STORE_KEY)!);
            expect(stored.state.config.channels).toEqual([JSON.parse(JSON.stringify(custom))]);
            expect(maximumConfigBytes).toBeLessThan(32 * 1024);
            expect(scopedLocalStorage.getItem(CONFIG_STORE_KEY)).not.toContain("media_generation_profile");
        } finally {
            quota.mockRestore();
        }
    });

    test("refresh restores the selected system model after invalidating the old catalog", async () => {
        loadCatalog = async () => multipleModels("first-model", "chosen-model");
        await refreshSystemChannels();
        chooseModel("platform::chosen-model");
        const response = deferred<unknown>();
        loadCatalog = () => response.promise;
        const pending = refreshSystemChannels();
        expect(modelNames()).toEqual(["custom-text"]);
        expect(useConfigStore.getState().config.textModel).not.toBe("platform::chosen-model");
        response.resolve(multipleModels("first-model", "chosen-model"));
        await pending;
        expect(useConfigStore.getState().config.textModel).toBe("platform::chosen-model");
        expect(useConfigStore.getState().config.model).toBe("platform::chosen-model");
    });

    test("failed refresh retains selection for retry without authorizing cached models", async () => {
        loadCatalog = async () => multipleModels("first-model", "chosen-model");
        await refreshSystemChannels();
        chooseModel("platform::chosen-model");
        loadCatalog = async () => {
            throw new Error("fixture unavailable");
        };
        await expect(refreshSystemChannels()).rejects.toThrow();
        expect(modelNames()).toEqual(["custom-text"]);
        await applyUserSession({ provider: "platform", user: user("account-a") });
        await expect(refreshSystemChannels()).rejects.toThrow();
        loadCatalog = async () => multipleModels("first-model", "chosen-model");
        await refreshSystemChannels();
        expect(useConfigStore.getState().config.textModel).toBe("platform::chosen-model");
    });

    test("all modality selections survive a reload without persisting the system catalog", async () => {
        const response = catalog("unused");
        const models = ["image", "video", "text", "audio"].flatMap((capability) => ["first", "chosen"].map((choice) => ({ ...catalog(`${capability}-${choice}`).channels[0].models[0], capability })));
        loadCatalog = async () => ({ ...response, channels: [{ ...response.channels[0], models }] });
        await refreshSystemChannels();
        const selections = { model: "platform::video-chosen", imageModel: "platform::image-chosen", videoModel: "platform::video-chosen", textModel: "platform::text-chosen", audioModel: "platform::audio-chosen" };
        for (const key of Object.keys(selections) as (keyof typeof selections)[]) useConfigStore.getState().updateConfig(key, selections[key]);
        const persisted = JSON.parse(scopedLocalStorage.getItem(CONFIG_STORE_KEY)!);
        expect(persisted.state.pendingModelSelections).toEqual(selections);
        expect(persisted.state.config.channels.map((channel: { id: string }) => channel.id)).toEqual(["custom"]);
        await applyUserSession({ provider: "platform", user: user("account-a") });
        await refreshSystemChannels();
        expect(useConfigStore.getState().config).toMatchObject(selections);
        expect(useConfigStore.getState().pendingModelSelections).toEqual({});
    });

    test("malformed persisted channel data cannot block the fresh catalog", async () => {
        scopedLocalStorage.setItem(CONFIG_STORE_KEY, JSON.stringify({ state: { config: { channels: "invalid" }, pendingModelSelections: { textModel: { model: "invalid" } } }, version: 0 }));
        await applyUserSession({ provider: "platform", user: user("account-a") });
        await refreshSystemChannels();
        expect(useModelCatalogStore.getState().status).toBe("ready");
        expect(modelNames()).toEqual(["fresh-model"]);
        expect(useConfigStore.getState().config.textModel).toBe("platform::fresh-model");
    });

    test("reopening the session restores only current account selections and custom channels", async () => {
        loadCatalog = async () => multipleModels("first-model", "chosen-model");
        await refreshSystemChannels();
        chooseModel("platform::chosen-model");
        await applyUserSession({ provider: "platform", user: user("account-b") });
        await refreshSystemChannels();
        expect(useConfigStore.getState().config.textModel).toBe("platform::first-model");
        expect(modelNames()).not.toContain("custom-text");
        chooseModel("platform::first-model");
        await applyUserSession({ provider: "platform", user: user("account-a") });
        await refreshSystemChannels();
        expect(useConfigStore.getState().config.textModel).toBe("platform::chosen-model");
        expect(useConfigStore.getState().config.channels.find((channel) => channel.id === "custom")).toEqual(custom);
        await applyUserSession({ provider: "platform", user: null });
        await applyUserSession({ provider: "platform", user: user("account-a") });
        await refreshSystemChannels();
        expect(useConfigStore.getState().config.textModel).toBe("platform::chosen-model");
    });

    test("a user selection made while loading wins over the previous system selection", async () => {
        loadCatalog = async () => multipleModels("first-model", "chosen-model");
        await refreshSystemChannels();
        chooseModel("platform::chosen-model");
        const response = deferred<unknown>();
        loadCatalog = () => response.promise;
        const pending = refreshSystemChannels();
        chooseModel("custom::custom-text");
        response.resolve(multipleModels("first-model", "chosen-model"));
        await pending;
        expect(useConfigStore.getState().config.textModel).toBe("custom::custom-text");
    });

    test("a successful catalog removing the selected model discards its pending preference", async () => {
        loadCatalog = async () => multipleModels("first-model", "chosen-model");
        await refreshSystemChannels();
        chooseModel("platform::chosen-model");
        loadCatalog = async () => ({ ...catalog("unused"), channels: [] });
        await refreshSystemChannels();
        expect(useConfigStore.getState().config.textModel).toBe("custom::custom-text");
        loadCatalog = async () => multipleModels("first-model", "chosen-model");
        await refreshSystemChannels();
        expect(useConfigStore.getState().config.textModel).toBe("custom::custom-text");
    });

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
