import { afterAll, expect, test } from "bun:test";
import { Window } from "happy-dom";
const dom = new Window({ url: "https://studio.example.test" });
for (const [key, value] of Object.entries({
    window: dom,
    document: dom.document,
    navigator: dom.navigator,
    HTMLElement: dom.HTMLElement,
    HTMLInputElement: dom.HTMLInputElement,
    HTMLTextAreaElement: dom.HTMLTextAreaElement,
    HTMLSelectElement: dom.HTMLSelectElement,
    NodeFilter: dom.NodeFilter,
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
const { appBasePath } = await import("@/lib/app-path");
const navigationPath = process.env.STUDIO_NAVIGATION_TEST_PATH || "/canvas/original-canvas";
const expectedReturnPath = navigationPath === "/" ? appBasePath || "/" : `${appBasePath}${navigationPath}`;
const { apiClient } = await import("@/services/api/request");
import type { StudioAccountSummary } from "@/services/api/studio-account";
afterAll(() => dom.happyDOM.abort());

test("account menu reads platform data and drops another user's response and failed balances", async () => {
    const client = new QueryClient();
    const old = apiClient.defaults.adapter;
    const host = document.createElement("div");
    document.body.append(host);
    const root = createRoot(host);
    const visits: { url: string; target?: string; features?: string }[] = [];
    const oldOpen = window.open;
    window.open = ((url: string | URL, target?: string, features?: string) => {
        visits.push({ url: String(url), target, features });
        return null;
    }) as typeof window.open;
    const account: StudioAccountSummary = {
        userId: "alice",
        displayName: "平台昵称",
        avatarUrl: "data:image/png;base64,AA==",
        workspaceId: "w1",
        workspaceName: "Personal space",
        workspaceKind: "personal",
        membershipGeneration: 0,
        membership: { state: "ready", active_count: 0 },
        benefits: { state: "ready", items: [] },
        spending: { state: "not_applicable" },
        capabilities: { can_read_finance: true, can_purchase: true },
        balance: { state: "ready", kind: "personal", available_points: "0.123456789012345" },
        applications: [],
        walletUrl: "https://console.example/console/wallet",
        usageUrl: "https://console.example/console/records",
    };
    const user = { id: "alice", username: "alice", displayName: "alice", status: "active", role: "user" } as const;
    const context = { userId: user.id, workspaceId: "w1", workspaceKind: "personal" as const, workspaceName: "Personal space", membershipGeneration: 0, supported: true };
    useUserStore.setState({ platformIdentity: true, user, workspaceContext: context });
    // The session read below the account mismatch uses the genuine session handler in production.
    apiClient.defaults.adapter = async (config) => ({
        config,
        headers: {},
        status: 200,
        statusText: "OK",
        data: {
            code: 0,
            data:
                config.url === "/auth/account-summary"
                    ? structuredClone(account)
                    : config.url === "/auth/workspaces"
                      ? {
                            userId: user.id,
                            items: [
                                { id: "w1", name: "Personal space", kind: "personal", membershipGeneration: 0, supported: true },
                                { id: "team_a", name: "Alpha", kind: "team", membershipGeneration: 2, supported: false },
                            ],
                        }
                      : { user: useUserStore.getState().user, provider: "platform" },
        },
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
                    <MemoryRouter initialEntries={[navigationPath]}>
                        <StudioAccountBar />
                    </MemoryRouter>
                </QueryClientProvider>,
            ),
        );
        await flush();
        expect(host.textContent).toContain("个人空间");
        expect(host.textContent).not.toContain("0.123456789012345");
        await act(async () => host.querySelector<HTMLButtonElement>(".ow-shared-account-trigger")!.click());
        await flush();
        const wallet = () => [...document.querySelectorAll<HTMLButtonElement>("button")].find((button) => button.textContent === "查看积分 ›");
        await act(async () => wallet()!.click());
        const walletTarget = new URL(visits[0].url, window.location.origin);
        expect(walletTarget.pathname).toBe(`${appBasePath}/api/auth/console-navigation`);
        expect(Object.fromEntries(walletTarget.searchParams)).toEqual({ destination: "wallet", return_path: expectedReturnPath, user_id: "alice", workspace_id: "w1" });
        expect(visits[0].target).toBe("_blank");
        expect(visits[0].features).toBe("noopener,noreferrer");
        expect(document.body.textContent).toContain("✦ 0.123456789012345");
        expect(document.body.textContent).toContain("暂无有效订阅");
        expect(host.querySelector("img")?.getAttribute("src")).toBe(account.avatarUrl!);
        const usage = [...document.querySelectorAll("a")].find((a) => a.textContent === "使用记录 ›")!;
        expect(new URL(usage.href).searchParams.get("destination")).toBe("usage");
        expect(new URL(usage.href).searchParams.get("return_path")).toBe(expectedReturnPath);
        const spaceTrigger = document.querySelector<HTMLButtonElement>('button[aria-label="切换空间：个人空间"]')!;
        await act(async () => spaceTrigger.click());
        await flush();
        const team = [...document.querySelectorAll<HTMLButtonElement>("button")].find((button) => button.textContent?.includes("Alpha"))!;
        expect(team.disabled).toBe(true);
        expect(team.textContent).toContain("Studio 团队画布暂未开放");
        expect(document.querySelector(".ow-shared-account-panel")?.contains(team)).toBe(false);
        await act(async () => useUserStore.setState({ user: { ...user, id: "bob", displayName: "Bob" }, workspaceContext: { ...context, userId: "bob", workspaceId: "w2" } }));
        await flush();
        expect(document.body.innerHTML).not.toContain("0.123456789012345");
        expect(document.body.innerHTML).not.toContain(account.avatarUrl!);
        expect(wallet()).toBeUndefined();
        account.userId = "bob";
        account.workspaceId = "w2";
        account.balance = { state: "unavailable", kind: "unavailable" };
        await act(async () => {
            await client.invalidateQueries({ queryKey: ["studio-account-summary", "bob"] });
        });
        await flush();
        await act(async () => host.querySelector<HTMLButtonElement>(".ow-shared-account-trigger")!.click());
        await flush();
        expect(document.body.textContent).toContain("暂时无法读取");
        expect(document.body.textContent).not.toContain("✦ 0");
        account.balance = { state: "ready", kind: "personal", available_points: "0" };
        await act(async () => {
            await client.invalidateQueries({ queryKey: ["studio-account-summary", "bob"] });
        });
        await flush();
        expect(document.body.textContent).toContain("✦ 0");
        account.membership = { state: "ready", plan_title: "创作会员" };
        account.capabilities = { can_read_finance: false, can_purchase: false };
        await act(async () => {
            await client.invalidateQueries({ queryKey: ["studio-account-summary", "bob"] });
        });
        await flush();
        expect(document.body.textContent).toContain("创作会员");
        expect(document.body.textContent).not.toContain("充值");
        expect(document.body.textContent).not.toContain("付款订单");
        account.benefits = { state: "unavailable" };
        account.spending = { state: "unavailable" };
        await act(async () => {
            await client.invalidateQueries({ queryKey: ["studio-account-summary", "bob"] });
        });
        await flush();
        expect(document.body.textContent).toContain("暂时无法读取可用权益");
        expect(document.body.textContent).toContain("暂时无法读取本人用量");
        expect(document.body.textContent).toContain("重新读取");
    } finally {
        await act(async () => root.unmount());
        client.clear();
        host.remove();
        apiClient.defaults.adapter = old;
        window.open = oldOpen;
    }
});

const { useCanvasKeyboard } = await import("@/pages/canvas/use-canvas-keyboard");
const { OceanAccountBar } = await import("@/vendor/oceanway-account-v1/account");
const { StudioProductSwitcher } = await import("@/components/layout/studio-product-switcher");
const { getStudioProductNavigation } = await import("@/services/api/studio-account");

test("product navigation preserves registered path and rejects late identity, generation and destination changes", async () => {
    const original = apiClient.defaults.adapter;
    const user = { id: "nav-user", username: "nav", displayName: "Nav", role: "user" as const, status: "active" as const, createdAt: "", updatedAt: "" };
    const context = { userId: user.id, workspaceId: "personal-nav", workspaceKind: "personal" as const, workspaceName: "个人空间", membershipGeneration: 0, supported: true };
    const account = { userId: user.id, workspaceId: context.workspaceId, membershipGeneration: 0, applications: [{ product: "drama", url: "https://apps.example/drama" }] } as StudioAccountSummary;
    let target = "https://apps.example/drama?workspace_id=personal-nav&membership_generation=0";
    let release: (() => void) | undefined;
    let deferred = false;
    apiClient.defaults.adapter = async (config) => {
        if (deferred)
            await new Promise<void>((resolve) => {
                release = resolve;
            });
        return { config, status: 200, statusText: "OK", headers: {}, data: { code: 0, data: { userId: user.id, workspaceId: context.workspaceId, membershipGeneration: 0, url: target } } };
    };
    try {
        useUserStore.setState({ user, workspaceContext: context });
        expect(await getStudioProductNavigation(account, "drama")).toBe(target);
        target = "https://apps.example/drama?workspace_id=another&membership_generation=0";
        await expect(getStudioProductNavigation(account, "drama")).rejects.toThrow();
        target = "https://unregistered.example/drama?workspace_id=personal-nav&membership_generation=0";
        await expect(getStudioProductNavigation(account, "drama")).rejects.toThrow();
        target = "https://apps.example/drama?workspace_id=personal-nav&membership_generation=0";
        deferred = true;
        const pending = getStudioProductNavigation(account, "drama");
        await new Promise((resolve) => setTimeout(resolve, 10));
        useUserStore.setState({ user: { ...user, id: "other" }, workspaceContext: { ...context, userId: "other" } });
        release!();
        await expect(pending).rejects.toThrow("账号或空间已变化");
        deferred = false;
        useUserStore.setState({ user, workspaceContext: { ...context, membershipGeneration: 2 } });
        await expect(getStudioProductNavigation(account, "drama")).rejects.toThrow("账号或空间已变化");
    } finally {
        apiClient.defaults.adapter = original;
    }
});

test.each(["切换产品 · OceanWay Studio", "工作空间 · 当前个人空间", "账户中心 · 账号", "切换 OceanWay 产品：Studio", "键盘账号 · 个人空间"])("Escape closes %s and returns focus before the canvas can leave focus mode", async (label) => {
    const host = document.createElement("div");
    document.body.append(host);
    const root = createRoot(host);
    const client = new QueryClient();
    const oldAdapter = apiClient.defaults.adapter;
    apiClient.defaults.adapter = async (config) => ({ config, status: 200, statusText: "OK", headers: {}, data: { code: 0, data: { userId: "keyboard", items: [] } } });
    useUserStore.setState({
        platformIdentity: false,
        user: { id: "keyboard", username: "keyboard", displayName: "键盘账号", role: "user", status: "active", createdAt: "", updatedAt: "" },
        workspaceContext: { userId: "keyboard", workspaceId: "keyboard-personal", workspaceKind: "personal", workspaceName: "个人空间", membershipGeneration: 0, supported: true },
    });
    let exits = 0,
        deselections = 0,
        edits = 0;
    const selected = { current: new Set(["existing-node"]) };
    const noop = () => {};
    function Canvas() {
        useCanvasKeyboard({
            nodesRef: { current: [] },
            selectedNodeIdsRef: selected,
            selectedConnectionId: null,
            setSelectedNodeIds: noop,
            setSelectedConnectionId: noop,
            setContextMenu: noop,
            setShortcutRequestNonce: noop,
            setInfoNodeId: noop,
            setCropNodeId: noop,
            setRegionEditNodeId: noop,
            setAnnotationNodeId: noop,
            saveCanvasProject: noop,
            zoomToActualSize: noop,
            fitCanvasContent: noop,
            fitCanvasSelection: noop,
            undoCanvas: () => {
                edits++;
            },
            redoCanvas: noop,
            cancelSelectionBox: noop,
            copySelectedNodes: noop,
            pasteCopiedNodes: () => {
                edits++;
                return false;
            },
            restoreCopiedNodesFromText: () => false,
            shouldPreferCopiedNodes: () => false,
            pasteSystemClipboard: () => {
                edits++;
                return false;
            },
            deleteNodes: () => {
                edits++;
            },
            deleteConnection: noop,
            deselectCanvas: () => {
                deselections++;
            },
            zoomCanvasIn: noop,
            zoomCanvasOut: noop,
            focusMode: true,
            exitFocusMode: () => {
                exits++;
            },
            toggleFocusMode: noop,
            onOpenSearch: noop,
            beginBatchConnection: noop,
        });
        if (label === "切换 OceanWay 产品：Studio") return <StudioProductSwitcher />;
        if (label === "键盘账号 · 个人空间") return <StudioAccountBar compact />;
        return (
            <OceanAccountBar
                compact
                productName="Studio"
                products={[{ name: "Console", href: "/console" }]}
                workspaceId="p"
                workspaces={[
                    { id: "p", name: "当前个人空间" },
                    { id: "t", name: "测试团队" },
                ]}
                onWorkspaceChange={noop}
                displayName="账号"
                balance={{ state: "ready", amount: "0" }}
                accountHref="/console/account"
            />
        );
    }
    const escape = async () => {
        await act(async () => document.activeElement!.dispatchEvent(new dom.KeyboardEvent("keydown", { key: "Escape", bubbles: true, cancelable: true })));
        await act(async () => {
            await new Promise((resolve) => setTimeout(resolve, 20));
        });
    };
    try {
        await act(async () =>
            root.render(
                <QueryClientProvider client={client}>
                    <MemoryRouter>
                        <Canvas />
                    </MemoryRouter>
                </QueryClientProvider>,
            ),
        );
        const trigger = host.querySelector<HTMLButtonElement>(`button[aria-label="${label}"]`)!;
        await act(async () => trigger.click());
        expect(trigger.getAttribute("aria-expanded")).toBe("true");
        if (label === "切换 OceanWay 产品：Studio" || label === "键盘账号 · 个人空间") {
            await act(async () => {
                document.activeElement!.dispatchEvent(new dom.KeyboardEvent("keydown", { key: "Delete", bubbles: true, cancelable: true }));
                document.activeElement!.dispatchEvent(new dom.KeyboardEvent("keydown", { key: "z", metaKey: true, bubbles: true, cancelable: true }));
                document.activeElement!.dispatchEvent(new dom.Event("paste", { bubbles: true, cancelable: true }));
            });
            expect(edits).toBe(0);
        }
        await escape();
        expect(exits).toBe(0);
        expect(deselections).toBe(0);
        expect(trigger.getAttribute("aria-expanded")).toBe("false");
        expect(document.activeElement).toBe(trigger);
        selected.current.clear();
        await escape();
        expect(exits).toBe(1);
    } finally {
        await act(async () => root.unmount());
        client.clear();
        apiClient.defaults.adapter = oldAdapter;
        host.remove();
    }
});
