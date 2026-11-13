import { afterAll, afterEach, expect, mock, test } from "bun:test";
import { Window } from "happy-dom";
import type { AuthSessionPayload } from "@/services/api/auth";

const dom = new Window({ url: "https://studio.example.test", settings: { navigation: { disableChildFrameNavigation: true } } });
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
const { MemoryRouter, Routes, Route, useLocation } = await import("react-router");
const { useUserStore } = await import("@/stores/use-user-store");
mock.module("@/lib/user-session", () => ({ applyUserSession: async ({ user }: AuthSessionPayload) => useUserStore.setState({ user, hydrated: true }) }));
const { default: LoginPage } = await import("@/pages/auth/login");
const { RequireAuth } = await import("@/components/auth/require-auth");
const { PlatformLoginDialogHost } = await import("@/components/auth/platform-login-dialog-host");
const { useAuthDialogStore } = await import("@/stores/use-auth-dialog-store");
const { apiClient } = await import("@/services/api/request");
const { getAuthSession, invalidateAuthSessionCache } = await import("@/services/api/auth");
const originalAdapter = apiClient.defaults.adapter;
const callback = dom.location.origin + "/platform/login/complete";
const origin = "https://platform.example.test";
const approved = origin + "/login?return=" + encodeURIComponent(callback);
const requests: string[] = [];
let loginURL = approved;
let authenticated = false;

function Location() {
    const { pathname, search, hash } = useLocation();
    return <output>{pathname + search + hash}</output>;
}

async function mount(path: string) {
    useUserStore.setState({ user: null, hydrated: true });
    apiClient.defaults.adapter = async (config) => {
        requests.push(config.url!);
        expect(config.withCredentials).toBe(true);
        const data = config.url === "/auth/login-url" ? { url: loginURL } : { provider: "platform", user: authenticated ? { id: "server-user" } : null };
        return { config, headers: {}, status: 200, statusText: "OK", data: { code: 0, data } };
    };
    const host = document.createElement("div");
    document.body.append(host);
    const root = createRoot(host);
    await act(async () =>
        root.render(
            <MemoryRouter initialEntries={[path]}>
                <Location />
                <Routes>
                    {["/login", "/register", "/forgot-password"].map((p) => (
                        <Route key={p} path={p} element={<LoginPage />} />
                    ))}
                    <Route
                        path="/canvas/:id"
                        element={
                            <RequireAuth>
                                <span>项目画布</span>
                            </RequireAuth>
                        }
                    />
                    <Route path="/" element={<PlatformLoginDialogHost />} />
                </Routes>
            </MemoryRouter>,
        ),
    );
    return async () => {
        await act(async () => root.unmount());
        host.remove();
    };
}

function notify(type: string, options: { origin?: string; source?: Window } = {}) {
    const frame = document.querySelector("iframe");
    window.dispatchEvent(
        new dom.MessageEvent("message", {
            data: { type, user: { id: "untrusted" } },
            origin: options.origin ?? origin,
            source: options.source ?? frame?.contentWindow,
        }) as unknown as MessageEvent,
    );
}

afterEach(() => {
    apiClient.defaults.adapter = originalAdapter;
    invalidateAuthSessionCache();
    requests.length = 0;
    authenticated = false;
    loginURL = approved;
    useAuthDialogStore.getState().closeDialog();
});
afterAll(() => dom.happyDOM.abort());

for (const path of ["/login", "/register", "/forgot-password", "/canvas/test?tab=1#node", "/"]) {
    test(path + " 直接加载共用表单，只接受当前 iframe 消息并从服务端恢复身份", async () => {
        const next = "/canvas/test?tab=1#node";
        const isEntry = ["/login", "/register", "/forgot-password"].includes(path);
        if (path === "/") useAuthDialogStore.getState().openDialog();
        const close = await mount(isEntry ? path + "?next=" + encodeURIComponent(next) : path);
        try {
            const frame = document.querySelector("iframe")!;
            const url = new URL(frame.src);
            expect(url.origin).toBe(origin);
            expect(url.searchParams.get("embed")).toBe("product");
            expect(url.searchParams.get("auth")).toBe(path === "/forgot-password" ? "forgot" : null);
            expect(url.searchParams.get("return")).toBe(callback);
            expect(requests).toEqual(["/auth/login-url"]);
            await act(async () => {
                notify("oceanway:login-complete", { origin: "https://elsewhere.test" });
                notify("oceanway:login-complete", { source: window });
                notify("oceanway:login-ready");
            });
            expect(frame.inert).toBe(false);
            expect(requests).toEqual(["/auth/login-url"]);
            await getAuthSession(); // 缓存尚未登录的会话。
            authenticated = true;
            await act(async () => {
                notify("oceanway:login-complete");
                notify("oceanway:login-complete");
            });
            expect(requests).toEqual(["/auth/login-url", "/auth/session", "/auth/session"]);
            expect(useUserStore.getState().user?.id).toBe("server-user");
            expect(document.querySelector("output")?.textContent).toBe(path === "/" ? "/" : next);
            expect(document.querySelector("iframe")).toBeNull();
        } finally {
            await close();
        }
    });
}

test("地址或会话确认失败时保持匿名，可以重试", async () => {
    loginURL = "http://platform.example.test/login";
    const close = await mount("/register");
    try {
        const retry = async () => {
            const button = Array.from(document.querySelectorAll("button")).find((item) => item.textContent?.replace(/\s/g, "") === "重试")!;
            await act(async () => button.click());
        };
        expect(document.querySelector("iframe")).toBeNull();
        loginURL = approved;
        await retry();
        await act(async () => notify("oceanway:login-complete"));
        expect(document.querySelector('[role="alert"]')?.textContent).toContain("未能确认");
        expect(useUserStore.getState().user).toBeNull();
        await retry();
        expect(document.querySelector("iframe")).toBeTruthy();
        await act(async () => notify("oceanway:login-dismiss"));
        expect(document.querySelector("output")?.textContent).toBe("/");
    } finally {
        await close();
    }
});
