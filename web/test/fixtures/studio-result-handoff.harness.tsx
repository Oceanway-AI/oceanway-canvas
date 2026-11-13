import { afterAll, expect, test } from "bun:test";
import { Window } from "happy-dom";

const dom = new Window({ url: "https://studio.example.test" });
for (const [key, value] of Object.entries({
    window: dom,
    document: dom.document,
    navigator: dom.navigator,
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
const { act } = await import("react");
const { createRoot } = await import("react-dom/client");
const { App } = await import("antd");
const { MediaResult } = await import("@/pages/create/creation-workspace-messages");
const { useUserStore } = await import("@/stores/use-user-store");
const { apiClient } = await import("@/services/api/request");
afterAll(() => dom.happyDOM.abort());

test("固定结果按 Resource 转到画布，也可单独采用为创作素材", async () => {
    useUserStore.setState({ platformIdentity: true });
    const writes: string[] = [];
    apiClient.defaults.adapter = async (config) => {
        let data: unknown = { items: [] };
        let code = 0;
        if (config.url === "/resources/existing") {
            data = { resource: { id: "existing", provider: "platform", status: "ready", kind: "image", width: 640, height: 480 } };
        } else if (config.url === "/assets/platform-existing") {
            if (config.method === "put") {
                writes.push(JSON.parse(config.data).asset.data.storageKey);
            } else {
                code = 404;
            }
        } else if (config.url !== "/resources/access") {
            throw new Error(`Unexpected request ${config.method} ${config.url}`);
        }
        return { config, headers: {}, status: 200, statusText: "OK", data: { code, data } };
    };
    const host = document.createElement("div");
    document.body.append(host);
    const root = createRoot(host);
    const selections: unknown[] = [];
    const noop = () => {};
    const render = (fixed: boolean) =>
        root.render(
            <App>
                <MediaResult
                    item={{
                        id: "selected-message",
                        role: "assistant",
                        mode: "image",
                        status: "done",
                        content: "已有图片",
                        createdAt: "2026-11-09T00:00:00Z",
                        ...(fixed ? { resultStorageKeys: ["resource:existing"] } : { resultUrls: ["https://expired.test/image.png"] }),
                    }}
                    onRetryFailure={noop}
                    onRefreshResult={noop}
                    onRecoverMedia={noop}
                    onCreateVariant={noop}
                    onContinueCanvas={(selection) => selections.push(selection)}
                    openingCanvas={false}
                />
            </App>,
        );
    const button = (label: string) => [...host.querySelectorAll<HTMLButtonElement>("button")].find((b) => b.textContent?.trim() === label)!;
    try {
        await act(async () => render(true));
        expect(button("添加到画布").disabled).toBe(false);
        await act(async () => button("添加到画布").click());
        expect(selections).toEqual([{ assetIds: [], resourceIds: ["existing"] }]);
        expect(writes).toEqual([]);
        await act(async () => button("加入创作素材").click());
        expect(writes).toEqual(["resource:existing"]);
        await act(async () => render(false));
        expect(button("添加到画布").disabled).toBe(true);
        expect(button("加入创作素材").disabled).toBe(true);
    } finally {
        await act(async () => root.unmount());
        host.remove();
    }
});
