import { afterAll, expect, test } from "bun:test";
import { Window } from "happy-dom";
const dom = new Window({ url: "https://studio.example.test" });
for (const [key, value] of Object.entries({
    window: dom,
    document: dom.document,
    navigator: dom.navigator,
    HTMLElement: dom.HTMLElement,
    HTMLAnchorElement: dom.HTMLAnchorElement,
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
localforage.getItem = async <T,>(key: string) => (storage.get(key) as T) ?? null;
localforage.setItem = async <T,>(key: string, value: T) => {
    storage.set(key, value);
    return value;
};
localforage.removeItem = async (key: string) => {
    storage.delete(key);
};
const { act } = await import("react");
const { createRoot } = await import("react-dom/client");
const { MemoryRouter } = await import("react-router");
const { QueryClient, QueryClientProvider } = await import("@tanstack/react-query");
const { App } = await import("antd");
const { default: CreatePage } = await import("@/pages/create");
const { defaultConfig, useConfigStore } = await import("@/stores/use-config-store");
const { useUserStore } = await import("@/stores/use-user-store");
const { useCreationPreferencesStore } = await import("@/stores/use-creation-preferences-store");
const { useModelCatalogStore } = await import("@/stores/use-model-catalog-store");
const { defaultModelCapabilityConfig, modelCapabilityConfigFor } = await import("@/lib/model-capabilities");
const { modelGroupReferenceLimits } = await import("@/lib/model-selection");
const { writeCreationConversationSnapshot } = await import("@/services/creation-conversation-snapshot");
const { getActiveUserScope } = await import("@/lib/user-scope");
const { apiClient } = await import("@/services/api/request");
apiClient.defaults.adapter = async (config) => {
    if (config.method === "get" && config.url === "/materials/assets") return { config, headers: {}, status: 200, statusText: "OK", data: { code: 0, data: { items: [], total: "0" } } };
    if (config.method === "get" && config.url === "/canvas-projects") return { config, headers: {}, status: 200, statusText: "OK", data: { code: 0, data: [] } };
    if (config.url !== "/public/inspirations/feed") throw new Error(`Unexpected request: ${config.method} ${config.url}`);
    return { config, headers: {}, status: 200, statusText: "OK", data: { code: 0, data: { enabled: false, entries: [], categories: {} } } };
};
globalThis.fetch = (async (url: string | URL | Request) => {
    if (!String(url).startsWith("/inspirations/")) throw new Error(`Unexpected fetch: ${url}`);
    return Response.json({ entries: [], sources: {} });
}) as typeof fetch;
afterAll(() => dom.happyDOM.abort());

for (const mode of ["text", "image", "video"] as const) {
    for (const available of [true, false]) {
        test(`真实首页 ${mode} 模式，平台目录${available ? "仅发布当前能力" : "尚未发布/加载"}`, async () => {
            const profile = defaultModelCapabilityConfig()[mode]!;
            const config = {
                ...defaultConfig,
                channels: available
                    ? [
                          {
                              id: "platform",
                              name: "平台",
                              baseUrl: "/api",
                              apiKey: "",
                              apiFormat: "openai" as const,
                              scope: "system" as const,
                              models: ["published"],
                              modelCosts: [
                                  {
                                      model: "published",
                                      capability: mode,
                                      unitPriceMicrocredits: 0,
                                      platform: { model_name: "published", model_type: mode, price: { status: "available", version: "p1", rates: { fixed: "0.1" } } },
                                      capabilityConfig: { version: 1 as const, [mode]: profile },
                                  },
                              ],
                          },
                      ]
                    : [],
                model: "platform::published",
                textModel: "platform::published",
                imageModel: "platform::published",
                videoModel: "platform::published",
                textModels: available && mode === "text" ? ["platform::published"] : [],
                imageModels: available && mode === "image" ? ["platform::published"] : [],
                videoModels: available && mode === "video" ? ["platform::published"] : [],
            };
            useConfigStore.setState({ config });
            useModelCatalogStore.setState({ status: "loading", error: null });
            useUserStore.setState({ platformIdentity: true, hydrated: true, user: { id: "catalog-user", username: "fixture", displayName: "Fixture", role: "user", status: "active" } });
            writeCreationConversationSnapshot(getActiveUserScope(), [{ id: "existing", title: "Existing", messages: [], updatedAt: "2026-11-08T00:00:00Z" }]);
            useCreationPreferencesStore.setState({ preferences: { mode }, hydrated: true });
            const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
            const host = document.createElement("div");
            document.body.append(host);
            const root = createRoot(host);
            try {
                await act(async () =>
                    root.render(
                        <QueryClientProvider client={client}>
                            <App>
                                <MemoryRouter>
                                    <CreatePage />
                                </MemoryRouter>
                            </App>
                        </QueryClientProvider>,
                    ),
                );
                expect(host.querySelector(".creation-home")).not.toBeNull();
                expect(host.textContent).not.toContain("当前模式暂无可用模型");
                await act(async () => useModelCatalogStore.setState({ status: "ready", error: null }));
                await act(async () => {
                    const editor = host.querySelector<HTMLTextAreaElement>("textarea.creation-chat-mention-editor")!;
                    Object.getOwnPropertyDescriptor(dom.HTMLTextAreaElement.prototype, "value")!.set!.call(editor, "A quiet ocean");
                    editor.dispatchEvent(new dom.Event("input", { bubbles: true }));
                });
                expect(host.querySelector<HTMLButtonElement>(".creation-submit")?.disabled).toBe(!available);
                if (available) {
                    const actual = modelCapabilityConfigFor(config, "platform::published");
                    for (const other of ["text", "image", "video"] as const) if (other !== mode) expect(actual[other]).toBeUndefined();
                    expect(host.textContent).not.toContain("当前模式暂无可用模型");
                } else {
                    expect(host.textContent).toContain("当前模式暂无可用模型");
                }
                for (const other of ["text", "image", "video"] as const) {
                    if (other === mode) continue;
                    await act(async () => host.querySelector<HTMLButtonElement>(`button[data-mode="${other}"]`)!.click());
                    expect(host.querySelector(".creation-home")).not.toBeNull();
                    expect(host.textContent).toContain("当前模式暂无可用模型");
                    expect(host.querySelector<HTMLButtonElement>(".creation-submit")?.disabled).toBe(true);
                }
            } finally {
                await act(async () => root.unmount());
                host.remove();
                client.clear();
            }
        });
    }
}

for (const logical of [true, false]) {
    test(`同名模型参考能力：${logical ? "逻辑路由合并" : "平台独立 SKU 隔离"}，真实首页随目录更新素材入口`, async () => {
        const baseProfile = defaultModelCapabilityConfig().video!;
        const references = { maxImages: 3, maxVideos: 2, maxAudios: 1 };
        const models = ["text-video", "reference-video"];
        const channel = {
            id: "platform",
            name: "平台",
            baseUrl: "/api",
            apiKey: "",
            apiFormat: "openai" as const,
            scope: "system" as const,
            models,
            modelCosts: models.map((model, index) => ({
                model,
                displayName: "同名视频模型",
                logicalModelId: logical ? model : undefined,
                capability: "video" as const,
                unitPriceMicrocredits: 0,
                platform: { model_name: model, model_type: "video", price: { status: "available", version: "p1", rates: { fixed: "0.1" } } },
                capabilityConfig: { version: 1 as const, video: { ...baseProfile, operations: [index ? "reference_to_video" : "text_to_video"], references: index ? references : { maxImages: 0, maxVideos: 0, maxAudios: 0 } } },
            })),
        };
        const config = { ...defaultConfig, channels: [channel], model: "platform::text-video", videoModel: "platform::text-video", textModels: [], imageModels: [], videoModels: models.map((model) => `platform::${model}`) };
        useConfigStore.setState({ config });
        useUserStore.setState({ platformIdentity: true, hydrated: true, user: { id: "catalog-user", username: "fixture", displayName: "Fixture", role: "user", status: "active" } });
        writeCreationConversationSnapshot(getActiveUserScope(), [{ id: "existing", title: "Existing", messages: [], updatedAt: "2026-11-08T00:00:00Z" }]);
        useCreationPreferencesStore.setState({ preferences: { mode: "video" }, hydrated: true });
        const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
        const host = document.createElement("div");
        document.body.append(host);
        const root = createRoot(host);
        try {
            await act(async () =>
                root.render(
                    <QueryClientProvider client={client}>
                        <App>
                            <MemoryRouter>
                                <CreatePage />
                            </MemoryRouter>
                        </App>
                    </QueryClientProvider>,
                ),
            );
            expect(host.querySelector(".creation-home")).not.toBeNull();
            expect(modelCapabilityConfigFor(config, config.videoModel).video!.references).toEqual({ maxImages: 0, maxVideos: 0, maxAudios: 0 });
            expect(modelGroupReferenceLimits(config, config.videoModel, "video")).toEqual(logical ? references : { maxImages: 0, maxVideos: 0, maxAudios: 0 });
            const libraryButton = host.querySelector<HTMLButtonElement>('button[aria-label="添加更多参考内容"]');
            if (logical) {
                expect(libraryButton).not.toBeNull();
                expect(libraryButton!.disabled).toBe(false);
            } else {
                expect(libraryButton).toBeNull();
            }
            await act(async () => useConfigStore.setState({ config: { ...config, channels: [{ ...channel, models: [models[0]], modelCosts: [channel.modelCosts[0]] }], videoModels: [config.videoModel] } }));
            expect(host.querySelector('button[aria-label="添加更多参考内容"]')).toBeNull();
            expect(host.querySelector(".creation-home")).not.toBeNull();
        } finally {
            await act(async () => root.unmount());
            host.remove();
            client.clear();
        }
    });
}

test.each([
    ["有效报价，一次点击提交且没有钱包上限", "new-token-price"],
    ["价格版本变化，未创建任务", "changed-price"],
])("原生视频生成按钮保留4秒720p参数：%s", async (_scenario, priceVersion) => {
    const { systemChannelModelChannels } = await import("@/lib/user-session");
    const { setActiveUserScope } = await import("@/lib/user-scope");
    const { PlatformBudgetConfirmationHost } = await import("@/components/layout/platform-budget-confirmation");
    const name = "doubao-seedance-2-0-fast-260128";
    const selected = `oceanway-platform::${name}`;
    const channels = systemChannelModelChannels([
        {
            id: "oceanway-platform",
            name: "OceanWay",
            displayName: "OceanWay",
            models: [
                {
                    id: name,
                    modelKey: name,
                    displayName: "Seedance",
                    protocol: "volcengine-ark-video",
                    capability: "video",
                    available: true,
                    priceTiers: [],
                    platform: {
                        model_name: name,
                        model_type: "video",
                        price: { status: "usage_dependent", version: "new-token-price", usage_schema: { tokens: { type: "number", unit: "token" }, resolution: { enum: ["480p", "720p"] }, video_input: { enum: ["none", "video"] } } },
                    },
                    capabilityConfig: {
                        version: 1,
                        video: {
                            ...defaultModelCapabilityConfig().video!,
                            ratios: ["16:9", "adaptive"],
                            defaultRatio: "adaptive",
                            resolutions: ["480p", "720p"],
                            defaultResolution: "720p",
                            duration: { selection: "enum", values: [-1, 4, 5, 6], default: 0 },
                        },
                    },
                },
            ],
        },
    ]);
    const userId = `direct-video-button-${priceVersion}`;
    setActiveUserScope(userId);
    useConfigStore.setState({ config: { ...defaultConfig, channels, model: selected, videoModel: selected, videoModels: [selected], imageModels: [], textModels: [] } });
    useUserStore.setState({
        platformIdentity: true,
        hydrated: true,
        features: { ...useUserStore.getState().features, creditsEnabled: false },
        user: { id: userId, username: "fixture", displayName: "Fixture", role: "user", status: "active" },
    });
    useCreationPreferencesStore.setState({ preferences: { mode: "video", video: { ratio: "16:9", seconds: "4", videoQuality: "720" } }, hydrated: true });
    writeCreationConversationSnapshot(getActiveUserScope(), [{ id: "direct", title: "Direct", messages: [], updatedAt: "2026-11-08T00:00:00Z" }]);
    const adapter = apiClient.defaults.adapter;
    const requests: Array<{ method?: string; url?: string; body: Record<string, unknown> }> = [];
    const task = { id: "direct-video", type: "canvas_video", status: "queued", progress: 0 };
    const currentPrice = priceVersion === "new-token-price";
    apiClient.defaults.adapter = async (config) => {
        if (config.url === "/public/inspirations/feed") return { config, headers: {}, status: 200, statusText: "OK", data: { code: 0, data: { enabled: false, entries: [], categories: {} } } };
        if (config.method === "get" && config.url === "/canvas-projects") return { config, headers: {}, status: 200, statusText: "OK", data: { code: 0, data: [] } };
        if (config.method === "get" && config.url === "/materials/assets") return { config, headers: {}, status: 200, statusText: "OK", data: { code: 0, data: { items: [], total: "0" } } };
        if (currentPrice && config.method === "get" && config.url === `/tasks/${task.id}`) return { config, headers: {}, status: 200, statusText: "OK", data: { code: 0, data: task } };
        requests.push({ method: config.method, url: config.url, body: JSON.parse(config.data || "{}") });
        if (currentPrice && config.method === "post" && config.url === "/tasks") return { config, headers: {}, status: 200, statusText: "OK", data: { code: 0, data: task } };
        if (config.method !== "post" || config.url !== "/tasks/preview") throw new Error(`Unexpected request: ${config.method} ${config.url}`);
        return {
            config,
            headers: {},
            status: 200,
            statusText: "OK",
            data: { code: 0, data: { allowed: true, combination_supported: true, previewUserId: userId, previewBinding: "binding", price_version: priceVersion, capability_revision: "a".repeat(64), connection_revision: 7, wallet_points: "0.123456789" } },
        };
    };
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const host = document.createElement("div");
    document.body.append(host);
    const root = createRoot(host);
    try {
        await act(async () =>
            root.render(
                <QueryClientProvider client={client}>
                    <App>
                        <PlatformBudgetConfirmationHost />
                        <MemoryRouter>
                            <CreatePage />
                        </MemoryRouter>
                    </App>
                </QueryClientProvider>,
            ),
        );
        await act(async () => {
            const editor = host.querySelector<HTMLTextAreaElement>("textarea.creation-chat-mention-editor")!;
            Object.getOwnPropertyDescriptor(dom.HTMLTextAreaElement.prototype, "value")!.set!.call(editor, "A quiet ocean");
            editor.dispatchEvent(new dom.Event("input", { bubbles: true }));
        });
        const submit = host.querySelector<HTMLButtonElement>(".creation-submit")!;
        expect(submit.disabled).toBe(false);
        expect(requests).toEqual([]);
        await act(async () => submit.click());
        const settled = () => (currentPrice ? requests.some((item) => item.url === "/tasks") : host.textContent?.includes("平台未批准当前参数或额度"));
        for (let i = 0; i < 100 && !settled(); i++) await act(async () => new Promise((resolve) => setTimeout(resolve, 10)));
        expect(requests.map((item) => item.url)).toEqual(currentPrice ? ["/tasks/preview", "/tasks"] : ["/tasks/preview"]);
        expect(requests[0]).toMatchObject({
            method: "post",
            url: "/tasks/preview",
            body: { type: "canvas_video", model: selected, platform: { expectedPriceVersion: "new-token-price" }, input: { config: { channelId: "oceanway-platform", model: name, size: "16:9", videoSeconds: "4", vquality: "720p" } } },
        });
        expect(requests[0].body.platform).not.toHaveProperty("walletMaxPoints");
        if (currentPrice) {
            expect(requests[1].method).toBe("post");
            expect(requests[1].body).toEqual({ ...requests[0].body, platform: { previewUserId: userId, previewBinding: "binding", expectedPriceVersion: priceVersion, expectedCapabilityRevision: "a".repeat(64), expectedConnectionRevision: 7 } });
            expect(document.querySelector('[role="dialog"]')).toBeNull();
        } else {
            expect(host.textContent).toContain("平台未批准当前参数或额度");
        }
    } finally {
        await act(async () => root.unmount());
        host.remove();
        client.clear();
        apiClient.defaults.adapter = adapter;
    }
});

test.each(["canvas_image", "canvas_video"])("%s previews and submits directly without a spending ceiling", async (type) => {
    const { createGenerationTask } = await import("@/services/api/task-center");
    const { http } = await import("@/services/api/request");
    const { spyOn } = await import("bun:test");
    const { setActiveUserScope } = await import("@/lib/user-scope");
    const oldState = useUserStore.getState();
    const oldScope = getActiveUserScope();
    const alice = { id: "direct-media-test-user", username: "alice", displayName: "Alice", role: "user", status: "active" } as const;
    setActiveUserScope(alice.id);
    useUserStore.setState({ platformIdentity: true, user: alice });
    const input = { prompt: "ocean", type, commandId: "direct-media", platform: { expectedPriceVersion: "p1" } };
    const quote = { previewUserId: alice.id, previewBinding: "bound-quote", capability_revision: "a".repeat(64), connection_revision: 7, price_version: "p1", wallet_points: "0.1234567890123456789", allowed: true, combination_supported: true };
    const post = spyOn(http, "post").mockResolvedValueOnce(quote).mockResolvedValueOnce({ id: "media", type, status: "queued" });
    try {
        const result = await createGenerationTask(input);
        expect(result.id).toBe("media");
        expect(post.mock.calls).toEqual([
            ["/tasks/preview", input],
            ["/tasks", { ...input, platform: { previewUserId: alice.id, previewBinding: "bound-quote", expectedPriceVersion: "p1", expectedCapabilityRevision: quote.capability_revision, expectedConnectionRevision: 7 } }],
        ]);
    } finally {
        post.mockRestore();
        useUserStore.setState(oldState);
        setActiveUserScope(oldScope);
    }
});

test("direct submission binds the displayed account, including when storage or cookies change before recovery", async () => {
    const { createGenerationTask } = await import("@/services/api/task-center");
    const { ApiError } = await import("@/services/api/request");
    const { setActiveUserScope } = await import("@/lib/user-scope");
    const oldState = useUserStore.getState();
    const oldScope = getActiveUserScope();
    const oldAdapter = apiClient.defaults.adapter;
    const alice = { id: "original-fixture-user", username: "alice", displayName: "Alice", role: "user", status: "active" } as const;
    const input = () => ({ commandId: "original-fixture-command", type: "canvas_image", prompt: "fixture", platform: { expectedPriceVersion: "p1", walletMaxPoints: "1" } });
    const quote = { price_version: "p1", capability_revision: "c".repeat(64), connection_revision: 2, wallet_points: "1", allowed: true, combination_supported: true, previewUserId: alice.id, previewBinding: "fixture-binding" };
    const paths: string[] = [];
    let expectedRecoveryUser: string | undefined;
    try {
        apiClient.defaults.adapter = async (config) => {
            paths.push(config.url ?? "");
            if (config.url === "/tasks/preview") return { config, headers: {}, status: 200, statusText: "OK", data: { code: 0, data: quote } };
            if (config.url === "/tasks") throw new Error("lost response");
            if (config.url === "/tasks/by-command/original-fixture-command") {
                expectedRecoveryUser = config.params?.expectedUserId;
                throw new ApiError("账号已切换", { status: 409, reason: "studio_preview_account_changed" });
            }
            throw new Error("unexpected route");
        };
        setActiveUserScope(alice.id);
        useUserStore.setState({ platformIdentity: false, user: null });
        await expect(createGenerationTask(input())).rejects.toThrow("账号已切换");
        expect(paths).toEqual([]);
        useUserStore.setState({ platformIdentity: true, user: alice });
        setActiveUserScope("different-fixture-user");
        await expect(createGenerationTask(input())).rejects.toThrow("账号已切换");
        expect(paths).toEqual([]);
        setActiveUserScope(alice.id);
        await expect(createGenerationTask(input())).rejects.toThrow("账号已切换");
        expect(expectedRecoveryUser).toBe(alice.id);
        expect(paths).toEqual(["/tasks/preview", "/tasks", "/tasks/by-command/original-fixture-command"]);
    } finally {
        apiClient.defaults.adapter = oldAdapter;
        useUserStore.setState(oldState);
        setActiveUserScope(oldScope);
    }
});
