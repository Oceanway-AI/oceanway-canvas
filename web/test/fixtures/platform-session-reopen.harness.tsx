import { afterAll, expect, test } from "bun:test";
import { Window } from "happy-dom";

const dom = new Window({ url: "https://studio.example.test/platform/login/complete" });
for (const [key, value] of Object.entries({
    window: dom,
    document: dom.document,
    navigator: dom.navigator,
    HTMLElement: dom.HTMLElement,
    HTMLAnchorElement: dom.HTMLAnchorElement,
    Element: dom.Element,
    Node: dom.Node,
    getComputedStyle: dom.getComputedStyle.bind(dom),
    IS_REACT_ACT_ENVIRONMENT: true,
})) {
    Object.defineProperty(globalThis, key, { configurable: true, writable: true, value });
}
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
const { AuthSessionHydrator } = await import("@/components/auth/auth-session-hydrator");
const { apiClient } = await import("@/services/api/request");
const { invalidateAuthSessionCache } = await import("@/services/api/auth");
const { useUserStore } = await import("@/stores/use-user-store");
const { useCreationPreferencesStore, CREATION_PREFERENCES_STORE_KEY } = await import("@/stores/use-creation-preferences-store");
const { setActiveUserScope, getActiveUserScope, scopedLocalStorage } = await import("@/lib/user-scope");
// Load the real hydration implementation before the deferred response so act
// can observe all subsequent storage and React updates in this isolated realm.
await import("@/lib/user-session");
const { resetRemoteUserDataSync } = await import("@/services/user-data-sync");
const { appQueryClient } = await import("@/lib/query-client");
const user = { id: "existing-user", username: "alice", displayName: "Alice", role: "user" as const, status: "active" as const, createdAt: "2026-11-08T00:00:00Z", updatedAt: "2026-11-08T00:00:00Z" };
afterAll(() => {
    resetRemoteUserDataSync();
    appQueryClient.clear();
    return dom.happyDOM.abort();
});

function Workspace() {
    const current = useUserStore((state) => state.user);
    return <div data-workspace>{current ? `workspace:${current.id}` : "anonymous"}</div>;
}

for (const knownInMemory of [false, true]) {
    test(`已有 Cookie 重开${knownInMemory ? "且保留身份内存" : "且身份内存为空"}仍执行 POST 初始化`, async () => {
        invalidateAuthSessionCache();
        setActiveUserScope(user.id);
        dom.document.cookie = "platform_session=existing; Secure; Path=/";
        scopedLocalStorage.setItem(CREATION_PREFERENCES_STORE_KEY, JSON.stringify({ state: { preferences: { mode: "video" } }, version: 0 }));
        useUserStore.setState({ user: knownInMemory ? user : null, hydrated: false });
        const requests: string[] = [];
        let releaseSession!: () => void;
        const sessionResponse = new Promise<void>((resolve) => {
            releaseSession = resolve;
        });
        apiClient.defaults.adapter = async (config) => {
            requests.push(`${config.method} ${config.url}`);
            expect(config.withCredentials).toBe(true);
            if (config.url === "/auth/session") {
                expect(config.method).toBe("post");
                expect(config.data).toBe("{}");
                expect(dom.document.cookie).toContain("platform_session=existing");
                await sessionResponse;
                return { config, status: 200, statusText: "OK", headers: {}, data: { code: 0, data: { provider: "platform", user } } };
            }
            if (config.url === "/model-catalog") return { config, status: 200, statusText: "OK", headers: {}, data: { code: 0, data: { source: "platform", channels: [] } } };
            throw new Error(`Unexpected request: ${config.method} ${config.url}`);
        };
        const host = document.createElement("div");
        document.body.append(host);
        const root = createRoot(host);
        let unsubscribe = () => {};
        try {
            await act(async () =>
                root.render(
                    <AuthSessionHydrator>
                        <Workspace />
                    </AuthSessionHydrator>,
                ),
            );
            expect(requests).toEqual(["post /auth/session"]);
            expect(host.querySelector("[data-workspace]")).toBeNull();
            const ready = new Promise<void>((resolve) => {
                unsubscribe = useUserStore.subscribe((state) => {
                    if (state.hydrated) resolve();
                });
            });
            await act(async () => {
                releaseSession();
                await ready;
            });
            expect(host.textContent).toBe(`workspace:${user.id}`);
            expect(useUserStore.getState().platformIdentity).toBe(true);
            expect(getActiveUserScope()).toBe(user.id);
            expect(useCreationPreferencesStore.getState().preferences.mode).toBe("video");
            expect(requests).toEqual(["post /auth/session", "get /model-catalog"]);
        } finally {
            unsubscribe();
            await act(async () => root.unmount());
            host.remove();
            invalidateAuthSessionCache();
            resetRemoteUserDataSync();
        }
    });
}
