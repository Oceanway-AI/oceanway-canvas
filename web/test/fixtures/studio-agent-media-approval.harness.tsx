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
const { ApprovalCard } = await import("@/components/canvas/canvas-cloud-agent-panel-parts");
const { canvasThemes } = await import("@/lib/canvas-theme");
afterAll(() => dom.happyDOM.abort());

test("平台审批卡先报价、保留精确积分、批准时不混入可编辑参数", async () => {
    const host = document.createElement("div");
    document.body.append(host);
    const root = createRoot(host);
    let quotes = 0,
        approvals = 0;
    const detail = { studioMedia: true, toolName: "generate_media", arguments: { mode: "image", channelId: "oceanway-platform", channelModelKey: "image-model", prompt: "测试", referenceNodeIds: [] } };
    const quote = { id: "quote-1", walletMaxPoints: "0.123456789", priceVersion: "v1", expiresAt: "2099-01-01T00:00:00Z", parameters: { size: "1:1", count: 1 } };
    const render = (quoted: boolean) =>
        root.render(
            <ApprovalCard
                approval={{ approvalId: "approval", detail: { ...detail, ...(quoted ? { studioQuote: quote } : {}) }, reason: "" }}
                theme={canvasThemes.dark}
                submitting={false}
                onReasonChange={() => {}}
                onQuote={() => {
                    quotes++;
                }}
                onApprove={(settings) => {
                    expect(settings).toBeUndefined();
                    approvals++;
                }}
                onReject={() => {}}
            />,
        );
    const button = (name: string) => [...host.querySelectorAll<HTMLButtonElement>("button")].find((b) => b.textContent?.trim() === name)!;
    try {
        await act(async () => render(false));
        expect(button("同意执行").disabled).toBe(true);
        await act(async () => button("获取平台报价").click());
        expect(quotes).toBe(1);
        expect(approvals).toBe(0);
        await act(async () => render(true));
        expect(host.textContent).toContain("0.123456789");
        expect(button("同意执行").disabled).toBe(false);
        await act(async () => button("同意执行").click());
        expect(approvals).toBe(1);
    } finally {
        await act(async () => root.unmount());
        host.remove();
    }
});
