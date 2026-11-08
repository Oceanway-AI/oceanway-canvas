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
const { defaultModelCapabilityConfig, modelCapabilityConfigFor } = await import("@/lib/model-capabilities");
const { writeCreationConversationSnapshot } = await import("@/services/creation-conversation-snapshot");
const { getActiveUserScope } = await import("@/lib/user-scope");
const { apiClient } = await import("@/services/api/request");
apiClient.defaults.adapter = async (config) => {
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
            useUserStore.setState({ platformIdentity: true, hydrated: true });
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
