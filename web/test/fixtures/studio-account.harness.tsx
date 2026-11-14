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
const { MemoryRouter } = await import("react-router");
const { QueryClient, QueryClientProvider } = await import("@tanstack/react-query");
const { StudioAccountBar } = await import("@/components/layout/studio-account-bar");
const { useUserStore } = await import("@/stores/use-user-store");
const { apiClient } = await import("@/services/api/request");
import type { StudioAccountSummary } from "@/services/api/studio-account";
afterAll(() => dom.happyDOM.abort());

test("account menu reads platform data and drops another user's response and failed balances", async () => {
    const client = new QueryClient();
    const old = apiClient.defaults.adapter;
    const host = document.createElement("div");
    document.body.append(host);
    const root = createRoot(host);
    const account: StudioAccountSummary = {
        userId: "alice",
        displayName: "平台昵称",
        avatarUrl: "data:image/png;base64,AA==",
        workspaceId: "w1",
        workspaceName: "Personal space",
        balance: { kind: "personal", available_points: "0.123456789012345" },
        applications: [],
        walletUrl: "https://console.example/console/wallet",
        usageUrl: "https://console.example/console/records",
    };
    const user = { id: "alice", username: "alice", displayName: "alice", status: "active", role: "user" } as const;
    useUserStore.setState({ platformIdentity: true, user });
    // The session read below the account mismatch uses the genuine session handler in production.
    apiClient.defaults.adapter = async (config) => ({
        config,
        headers: {},
        status: 200,
        statusText: "OK",
        data: { code: 0, data: config.url === "/auth/account-summary" ? structuredClone(account) : { user: useUserStore.getState().user, provider: "platform" } },
    });
    async function flush() {
        await act(async () => {
            await new Promise((r) => setTimeout(r, 20));
        });
    }
    try {
        await act(async () =>
            root.render(
                <QueryClientProvider client={client}>
                    <MemoryRouter>
                        <StudioAccountBar />
                    </MemoryRouter>
                </QueryClientProvider>,
            ),
        );
        await flush();
        const wallet = () => host.querySelector('a[title="OceanWay 通用余额"]');
        expect(wallet()?.getAttribute("href")).toBe(account.walletUrl!);
        expect(wallet()?.textContent).toContain("✦ 0.123456789012345");
        expect(host.textContent).toContain("个人空间");
        expect(host.querySelector("img")?.getAttribute("src")).toBe(account.avatarUrl!);
        await act(async () => host.querySelector<HTMLButtonElement>('button[aria-label="账户中心 · 平台昵称"]')!.click());
        expect([...document.querySelectorAll("a")].some((a) => a.href === account.usageUrl)).toBe(true);
        await act(async () => useUserStore.setState({ user: { ...user, id: "bob", displayName: "Bob" } }));
        await flush();
        expect(host.innerHTML).not.toContain("0.123456789012345");
        expect(host.innerHTML).not.toContain(account.avatarUrl!);
        expect(wallet()).toBeNull();
        account.userId = "bob";
        account.balance = { kind: "unavailable" };
        await act(async () => {
            await client.invalidateQueries({ queryKey: ["studio-account-summary", "bob"] });
        });
        await flush();
        expect(host.textContent).toContain("暂时无法读取");
        expect(host.textContent).not.toContain("✦ 0");
        account.balance = { kind: "personal", available_points: "0" };
        await act(async () => {
            await client.invalidateQueries({ queryKey: ["studio-account-summary", "bob"] });
        });
        await flush();
        expect(host.textContent).toContain("✦ 0");
    } finally {
        await act(async () => root.unmount());
        client.clear();
        host.remove();
        apiClient.defaults.adapter = old;
    }
});
