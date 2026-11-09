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
const { MediaResult } = await import("@/pages/create/creation-workspace-messages");
const { useUserStore } = await import("@/stores/use-user-store");
const { apiClient } = await import("@/services/api/request");
afterAll(() => dom.happyDOM.abort());

test("固定结果按钮按消息显式采用；URL-only 结果仍拒绝", async () => {
    useUserStore.setState({ platformIdentity: true });
    // No network is allowed: resolving display access is not an adoption.
    apiClient.defaults.adapter = async (config) => ({ config, headers: {}, status: 200, statusText: "OK", data: { code: 0, data: { items: [] } } });
    const host = document.createElement("div");
    document.body.append(host);
    const root = createRoot(host);
    const selections: unknown[] = [];
    const noop = () => {};
    const render = (fixed: boolean) =>
        root.render(
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
                onContinueCanvas={(ids, messageId) => selections.push({ ids, messageId })}
                openingCanvas={false}
            />,
        );
    const button = () => [...host.querySelectorAll<HTMLButtonElement>("button")].find((b) => b.textContent?.trim() === "添加到画布")!;
    try {
        await act(async () => render(true));
        expect(button().disabled).toBe(false);
        await act(async () => button().click());
        expect(selections).toEqual([{ ids: undefined, messageId: "selected-message" }]);
        await act(async () => render(false));
        expect(button().disabled).toBe(true);
    } finally {
        await act(async () => root.unmount());
        host.remove();
    }
});
