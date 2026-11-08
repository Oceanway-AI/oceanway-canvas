import { afterAll, afterEach, expect, test } from "bun:test";
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
const { MemoryRouter, Routes, Route, useSearchParams } = await import("react-router");
const { App } = await import("antd");
const { default: LoginPage } = await import("@/pages/auth/login");
const { default: RegisterPage } = await import("@/pages/auth/register");
const { default: ForgotPasswordPage } = await import("@/pages/auth/forgot-password");
const { AuthPanel } = await import("@/components/auth/auth-panel");
const { apiClient } = await import("@/services/api/request");
const { consumePlatformLoginNext } = await import("@/lib/platform-login");
const { useUserStore } = await import("@/stores/use-user-store");
const originalAdapter = apiClient.defaults.adapter;
const redirects: string[] = [];
Object.defineProperty(dom.location, "assign", { value: (url: string) => redirects.push(url) });

afterEach(() => {
    apiClient.defaults.adapter = originalAdapter;
    redirects.length = 0;
    dom.sessionStorage.clear();
});
afterAll(() => dom.happyDOM.abort());

function RegistrationDialog() {
    const [params] = useSearchParams();
    return (
        <AuthPanel
            variant="dialog"
            view="register"
            next={params.get("next") || "/"}
            onViewChange={() => {
                throw new Error("Unexpected local auth view");
            }}
            onAuthenticated={() => {
                throw new Error("Unexpected local authentication");
            }}
        />
    );
}

async function mount(path: string) {
    useUserStore.setState({ user: null, hydrated: true });
    const host = document.createElement("div");
    document.body.append(host);
    const root = createRoot(host);
    await act(async () =>
        root.render(
            <App>
                <MemoryRouter initialEntries={[path]}>
                    <Routes>
                        <Route path="/login" element={<LoginPage />} />
                        <Route path="/register" element={<RegisterPage />} />
                        <Route path="/forgot-password" element={<ForgotPasswordPage />} />
                        <Route path="/dialog" element={<RegistrationDialog />} />
                    </Routes>
                </MemoryRouter>
            </App>,
        ),
    );
    return {
        host,
        button: () => host.querySelector<HTMLButtonElement>("button")!,
        close: async () => {
            await act(async () => root.unmount());
            host.remove();
        },
    };
}

for (const path of ["/login", "/register", "/forgot-password", "/dialog"]) {
    test(`${path} 真实页面只有平台登录，点击后保留安全回跳`, async () => {
        const requests: string[] = [];
        apiClient.defaults.adapter = async (config) => {
            requests.push(`${config.method} ${config.url}`);
            expect(config.withCredentials).toBe(true);
            expect(config.data).toBeUndefined();
            return { config, headers: {}, status: 200, statusText: "OK", data: { code: 0, data: { url: "https://platform.example.test/login?return=registered" } } };
        };
        const page = await mount(`${path}?next=${encodeURIComponent("/canvas/existing?tab=details")}`);
        try {
            expect(page.host.textContent).toContain("使用 OceanWay 账号登录 Studio");
            expect(page.host.querySelectorAll("input").length).toBe(0);
            expect(page.host.querySelector('[role="checkbox"]')).toBeNull();
            expect(page.host.textContent).not.toContain("服务协议内容待补充");
            expect(requests).toEqual([]);
            await act(async () => page.button().click());
            expect(requests).toEqual(["get /auth/login-url"]);
            expect(redirects).toEqual(["https://platform.example.test/login?return=registered"]);
            expect(consumePlatformLoginNext()).toBe("/canvas/existing?tab=details");
            expect(consumePlatformLoginNext()).toBe("/");
        } finally {
            await page.close();
        }
    });
}

for (const path of ["/forgot-password", "/register", "/dialog"]) {
    test(`${path} 平台失败或不安全登录地址不跳转、不调用旧认证接口，允许重试`, async () => {
        const requests: string[] = [];
        let loginURL = "http://platform.example.test/login";
        let code = 503;
        apiClient.defaults.adapter = async (config) => {
            requests.push(`${config.method} ${config.url}`);
            return { config, headers: {}, status: 200, statusText: "OK", data: { code, msg: "fixture unavailable", data: { url: loginURL } } };
        };
        const page = await mount(`${path}?next=https://external.example.test`);
        try {
            for (const unsafe of ["", "http://platform.example.test/login", "https://user:pass@platform.example.test/login", "javascript:alert(1)"]) {
                loginURL = unsafe;
                await act(async () => page.button().click());
                expect(page.host.querySelector('[role="alert"]')?.textContent).toContain("暂时无法登录");
                expect(redirects).toEqual([]);
                expect(dom.sessionStorage.length).toBe(0);
                expect(page.host.querySelectorAll("input").length).toBe(0);
                expect(page.button().disabled).toBe(false);
                code = 0;
            }
            loginURL = "https://platform.example.test/login";
            await act(async () => page.button().click());
            expect(redirects).toEqual([loginURL]);
            expect(consumePlatformLoginNext()).toBe("/");
            expect(requests).toEqual(Array(5).fill("get /auth/login-url"));
        } finally {
            await page.close();
        }
    });
}
