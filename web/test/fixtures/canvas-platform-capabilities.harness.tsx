import { afterAll, expect, test } from "bun:test";
import { Window } from "happy-dom";

const dom = new Window({ url: "https://studio.example.test" });
for (const [key, value] of Object.entries({
    window: dom,
    document: dom.document,
    navigator: dom.navigator,
    HTMLElement: dom.HTMLElement,
    HTMLTextAreaElement: dom.HTMLTextAreaElement,
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
const { act } = await import("react");
const { createRoot } = await import("react-dom/client");
const { App } = await import("antd");
const { QueryClient, QueryClientProvider } = await import("@tanstack/react-query");
const { MemoryRouter } = await import("react-router");
const { CanvasNodePromptPanel, buildNodeConfig } = await import("@/components/canvas/canvas-node-prompt-panel");
const { defaultConfig, useConfigStore } = await import("@/stores/use-config-store");
const { useUserStore } = await import("@/stores/use-user-store");
const { defaultModelCapabilityConfig } = await import("@/lib/model-capabilities");
const { CanvasNodeType } = await import("@/types/canvas");
const { apiClient } = await import("@/services/api/request");
const { refreshSystemChannels } = await import("@/lib/user-session");
apiClient.defaults.adapter = async (config) => {
    throw new Error(`Unexpected request: ${config.method} ${config.url}`);
};
afterAll(() => dom.happyDOM.abort());

for (const workspaceMode of ["simple", "professional"] as const) {
    test(`恢复 ${workspaceMode} 画布的摘要模型后自动加载详情`, async () => {
        const modelName = "saved-video";
        const model = {
            id: modelName,
            modelKey: modelName,
            displayName: "Saved video",
            capability: "video",
            available: true,
            priceTiers: [],
            pricingMode: "platform",
            priceLabel: "",
            platform: { model_name: modelName, model_type: "video", state: "summary", price: { status: "available", version: "p1" }, price_version: "p1", capability_revision: "c1", connection_revision: 1 },
        };
        let resolveDetail!: (value: unknown) => void;
        const detail = new Promise((resolve) => {
            resolveDetail = resolve;
        });
        let resolveSummary!: (value: unknown) => void;
        const summary = new Promise((resolve) => {
            resolveSummary = resolve;
        });
        let reads = 0;
        apiClient.defaults.adapter = async (config) => {
            let data: unknown;
            if (config.url === "/model-catalog/summaries") data = await summary;
            else if (config.url === "/model-catalog/detail") {
                reads++;
                data = await detail;
            } else throw new Error(`Unexpected request: ${config.method} ${config.url}`);
            return { config, status: 200, statusText: "OK", headers: {}, data: { code: 0, data } };
        };
        useConfigStore.setState({ config: { ...defaultConfig, channels: [], videoModel: `oceanway-platform::${modelName}` }, pendingModelSelections: {} });
        useUserStore.setState({
            user: { id: `lazy-${workspaceMode}`, username: "fixture", displayName: "fixture", role: "user", status: "active", createdAt: "", updatedAt: "" },
            platformIdentity: true,
            hydrated: true,
            features: { ...useUserStore.getState().features, creditsEnabled: false },
        });
        const catalogLoading = refreshSystemChannels();
        const node = { id: "saved-node", title: "镜头", type: CanvasNodeType.Video, position: { x: 0, y: 0 }, width: 320, height: 180, metadata: { model: `oceanway-platform::${modelName}`, prompt: "Keep this prompt" } };
        const host = document.createElement("div");
        document.body.append(host);
        const root = createRoot(host);
        const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
        try {
            await act(async () =>
                root.render(
                    <QueryClientProvider client={client}>
                        <App>
                            <MemoryRouter>
                                <CanvasNodePromptPanel projectId="existing-canvas" node={node} workspaceMode={workspaceMode} isRunning={false} onPromptChange={() => {}} onConfigChange={() => {}} onGenerate={() => {}} />
                            </MemoryRouter>
                        </App>
                    </QueryClientProvider>,
                ),
            );
            expect(reads).toBe(0);
            expect(host.textContent).not.toContain("当前模式暂无可用模型");
            expect(host.querySelector<HTMLButtonElement>(".canvas-node-composer-submit")!.disabled).toBe(true);
            await act(async () => {
                resolveSummary({ source: "platform", catalogRevision: "r1", channels: [{ id: "oceanway-platform", name: "平台", models: [model] }] });
                await catalogLoading;
            });
            expect(reads).toBe(1);
            expect(host.textContent).toContain("正在加载模型参数");
            expect(host.textContent).not.toContain("当前模式暂无可用模型");
            expect(host.querySelector<HTMLButtonElement>(".canvas-node-composer-submit")!.disabled).toBe(true);
            await act(async () => {
                resolveDetail({ model: { ...model, platform: { ...model.platform, state: "detail" }, capabilityConfig: { version: 1, video: defaultModelCapabilityConfig().video } } });
                await detail;
            });
            expect(reads).toBe(1);
            expect(host.textContent).not.toContain("正在加载模型参数");
            expect(host.textContent).not.toContain("当前模式暂无可用模型");
            expect(host.querySelector<HTMLButtonElement>(".canvas-node-composer-submit")!.disabled).toBe(false);
        } finally {
            await act(async () => root.unmount());
            host.remove();
            client.clear();
        }
    });
}

for (const mode of ["image", "video"] as const) {
    for (const available of [true, false]) {
        for (const restored of [true, false]) {
            test(`真实画布 ${mode} ${restored ? "恢复旧模型" : "添加空节点"}：目录${available ? "发布对应能力" : "只有文本"}`, async () => {
                const modes = available ? ["text" as const, mode] : ["text" as const];
                const config = {
                    ...defaultConfig,
                    model: "oceanway-platform::text",
                    textModel: "oceanway-platform::text",
                    imageModel: "oceanway-platform::image",
                    videoModel: "oceanway-platform::video",
                    textModels: ["oceanway-platform::text"],
                    imageModels: available && mode === "image" ? ["oceanway-platform::image"] : [],
                    videoModels: available && mode === "video" ? ["oceanway-platform::video"] : [],
                    channels: [
                        {
                            id: "oceanway-platform",
                            name: "平台",
                            baseUrl: "/api",
                            apiKey: "",
                            apiFormat: "openai" as const,
                            scope: "system" as const,
                            models: modes,
                            modelCosts: modes.map((kind) => ({
                                model: kind,
                                capability: kind,
                                unitPriceMicrocredits: 0,
                                platform: { model_name: kind, model_type: kind, price: { status: "available", version: "p1" } },
                                capabilityConfig: { version: 1 as const, [kind]: defaultModelCapabilityConfig()[kind] },
                            })),
                        },
                    ],
                };
                useConfigStore.setState({ config });
                useUserStore.setState({ platformIdentity: true, hydrated: true, features: { ...useUserStore.getState().features, creditsEnabled: false } });
                const node = {
                    id: "node",
                    title: "镜头",
                    type: mode === "image" ? CanvasNodeType.Image : CanvasNodeType.Video,
                    position: { x: 0, y: 0 },
                    width: 320,
                    height: 180,
                    metadata: { prompt: "A quiet ocean", ...(restored ? { model: "removed-channel::retired-model" } : {}) },
                };
                const generated: unknown[][] = [];
                const host = document.createElement("div");
                document.body.append(host);
                const root = createRoot(host);
                const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
                try {
                    await act(async () =>
                        root.render(
                            <QueryClientProvider client={client}>
                                <App>
                                    <MemoryRouter>
                                        <CanvasNodePromptPanel projectId="existing-canvas" node={node} isRunning={false} onPromptChange={() => {}} onConfigChange={() => {}} onGenerate={(...args) => generated.push(args)} />
                                    </MemoryRouter>
                                </App>
                            </QueryClientProvider>,
                        ),
                    );
                    const button = host.querySelector<HTMLButtonElement>(".canvas-node-composer-submit")!;
                    expect(button).not.toBeNull();
                    expect(button.disabled).toBe(!available);
                    await act(async () => button.click());
                    expect(generated).toEqual(available ? [["node", mode, "A quiet ocean"]] : []);
                    if (available) expect(buildNodeConfig(config, node, mode, { capability: mode }).model).toBe(`oceanway-platform::${mode}`);
                    else expect(host.textContent).toContain("当前模式暂无可用模型");
                } finally {
                    await act(async () => root.unmount());
                    host.remove();
                    client.clear();
                }
            });
        }
    }
}
