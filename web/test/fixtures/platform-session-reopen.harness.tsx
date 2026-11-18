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
            if (config.url === "/auth/workspace-context")
                return { config, status: 200, statusText: "OK", headers: {}, data: { code: 0, data: { userId: user.id, workspaceId: "personal-101", workspaceKind: "personal", workspaceName: "Personal", membershipGeneration: 0, supported: true } } };
            if (config.url === "/model-catalog/summaries") return { config, status: 200, statusText: "OK", headers: {}, data: { code: 0, data: { source: "platform", catalogRevision: "catalog-v1", channels: [], hasMore: false } } };
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
            expect(requests).toEqual(["post /auth/session", "get /auth/workspace-context", "get /model-catalog/summaries"]);
        } finally {
            unsubscribe();
            await act(async () => root.unmount());
            host.remove();
            invalidateAuthSessionCache();
            resetRemoteUserDataSync();
        }
    });
}

test("a visible Studio window confirms focus against the live session and never persists the old tab into another user's scope", async () => {
    invalidateAuthSessionCache();
    const alice = { ...user, id: "fixture-alice" },
        bob = { ...user, id: "fixture-bob" };
    let actual = alice,
        reads = 0;
    const previous = apiClient.defaults.adapter;
    apiClient.defaults.adapter = async (config) => {
        if (config.url === "/auth/session") reads++;
        if (config.url === "/auth/workspace-context")
            return {
                config,
                headers: {},
                status: 200,
                statusText: "OK",
                data: { code: 0, data: { userId: actual.id, workspaceId: "personal-" + actual.id, workspaceKind: "personal", workspaceName: "Personal", membershipGeneration: 0, supported: true } },
            };
        return { config, headers: {}, status: 200, statusText: "OK", data: { code: 0, data: config.url === "/auth/session" ? { provider: "platform", user: actual } : { source: "platform", channels: [] } } };
    };
    const host = document.createElement("div");
    const root = createRoot(host);
    const flush = () =>
        act(async () => {
            await new Promise((r) => setTimeout(r, 45));
        });
    try {
        await act(async () =>
            root.render(
                <AuthSessionHydrator>
                    <span>private workspace</span>
                </AuthSessionHydrator>,
            ),
        );
        await flush();
        expect(getActiveUserScope()).toBe(alice.id);
        actual = bob;
        window.localStorage.setItem("yingce:active-user-scope", bob.id);
        expect(getActiveUserScope()).toBe(alice.id);
        scopedLocalStorage.setItem("draft-ownership", "belongs-to-alice");
        expect(window.localStorage.getItem("draft-ownership:user:" + bob.id)).toBeNull();
        await act(async () => {
            window.dispatchEvent(new dom.Event("focus"));
            window.dispatchEvent(new dom.Event("pageshow"));
        });
        await flush();
        expect(useUserStore.getState().user?.id).toBe(bob.id);
        expect(getActiveUserScope()).toBe(bob.id);
        expect(reads).toBe(2);
        // A delayed response from an earlier foreground transition must not switch back to Alice.
        let releaseLate!: () => void;
        const late = new Promise<void>((resolve) => {
            releaseLate = resolve;
        });
        let started = false;
        const immediate = apiClient.defaults.adapter as (config: import("axios").InternalAxiosRequestConfig) => Promise<import("axios").AxiosResponse>;
        apiClient.defaults.adapter = async (config) => {
            if (config.url === "/auth/session" && !started) {
                started = true;
                await late;
                return { config, headers: {}, status: 200, statusText: "OK", data: { code: 0, data: { provider: "platform", user: alice } } };
            }
            return immediate(config);
        };
        await act(async () => {
            window.dispatchEvent(new dom.Event("focus"));
        });
        expect(started).toBe(true);
        await act(async () => {
            window.dispatchEvent(new dom.StorageEvent("storage", { key: "yingce:active-user-scope", newValue: bob.id }));
        });
        await flush();
        releaseLate();
        await flush();
        expect(useUserStore.getState().user?.id).toBe(bob.id);
        expect(reads).toBe(3);
        const { ApiError } = await import("@/services/api/request");
        apiClient.defaults.adapter = async () => {
            throw new ApiError("登录已失效", { status: 401 });
        };
        await act(async () => {
            window.dispatchEvent(new dom.Event("focus"));
        });
        await flush();
        expect(useUserStore.getState().user).toBeNull();
        expect(getActiveUserScope()).toBe("guest");
    } finally {
        await act(async () => root.unmount());
        apiClient.defaults.adapter = previous;
        invalidateAuthSessionCache();
        resetRemoteUserDataSync();
    }
});
