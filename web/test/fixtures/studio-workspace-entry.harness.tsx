import { afterAll, expect, test } from "bun:test";
import { Window } from "happy-dom";

const dom = new Window({ url: "https://studio.example.test/?workspace_id=team_alpha" });
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
const reads: string[] = [];
localforage.getItem = async <T,>(key: string): Promise<T | null> => {
    reads.push(key);
    return null;
};
localforage.setItem = async <T,>(_key: string, value: T) => value;
localforage.removeItem = async () => {};
const { act } = await import("react");
const { createRoot } = await import("react-dom/client");
const { AuthSessionHydrator } = await import("@/components/auth/auth-session-hydrator");
const { apiClient } = await import("@/services/api/request");
const { invalidateAuthSessionCache } = await import("@/services/api/auth");
const { useUserStore } = await import("@/stores/use-user-store");
const { CREATION_PREFERENCES_STORE_KEY, useCreationPreferencesStore } = await import("@/stores/use-creation-preferences-store");
const { setActiveUserScope, scopedLocalStorage, getActiveUserScope } = await import("@/lib/user-scope");
const { resetRemoteUserDataSync } = await import("@/services/user-data-sync");
const { appQueryClient } = await import("@/lib/query-client");
const { applyUserSession } = await import("@/lib/user-session");
const { getStudioAccountSummary } = await import("@/services/api/studio-account");
const { getStudioWorkspaceContext } = await import("@/services/api/studio-workspace");
const { rememberPlatformLoginNext, clearPlatformLoginNext } = await import("@/lib/platform-login");
const user = { id: "fixture-alice", username: "alice", displayName: "Alice", role: "user" as const, status: "active" as const, createdAt: "", updatedAt: "" };
const personal = { userId: user.id, workspaceId: "personal-101", workspaceKind: "personal", workspaceName: "Personal", membershipGeneration: 0, supported: true };
afterAll(() => {
    resetRemoteUserDataSync();
    appQueryClient.clear();
    return dom.happyDOM.abort();
});
const flush = () =>
    act(async () => {
        await new Promise((resolve) => setTimeout(resolve, 60));
    });

test("real axios serialization preserves duplicate intent and login returns preserve object context", async () => {
    const urls: URL[] = [];
    apiClient.defaults.adapter = async (config) => {
        urls.push(new URL(apiClient.getUri(config), window.location.origin));
        return { config, status: 200, statusText: "OK", headers: {}, data: { code: 0, data: personal } };
    };
    try {
        dom.location.href = "https://studio.example.test/?workspace_id=team_alpha&workspace_id=team_beta";
        await getStudioWorkspaceContext();
        expect(urls[0].searchParams.getAll("workspace_id")).toEqual(["team_alpha", "team_beta"]);
        expect(urls[0].searchParams.has("workspace_id[]")).toBe(false);
        rememberPlatformLoginNext("/canvas/original?workspace_id=team_alpha");
        dom.location.href = "https://studio.example.test/platform/login/complete";
        await getStudioWorkspaceContext();
        expect(urls[1].searchParams.get("canvas_id")).toBe("original");
        expect(urls[1].searchParams.get("workspace_id")).toBe("team_alpha");
    } finally {
        clearPlatformLoginNext();
        dom.location.href = "https://studio.example.test/?workspace_id=team_alpha";
    }
});

test("team intent blocks personal cache and children; retry rechecks authority", async () => {
    invalidateAuthSessionCache();
    setActiveUserScope(user.id);
    scopedLocalStorage.setItem(CREATION_PREFERENCES_STORE_KEY, JSON.stringify({ state: { preferences: { mode: "video" } }, version: 0 }));
    useUserStore.setState({ user: null, hydrated: false, workspaceError: null });
    reads.length = 0;
    let attempt = 0;
    apiClient.defaults.adapter = async (config) => {
        let data: unknown;
        if (config.url === "/auth/session") data = { provider: "platform", user };
        else if (config.url === "/auth/workspace-context") {
            expect(config.params.get("workspace_id")).toBe("team_alpha");
            attempt++;
            data = { ...personal, workspaceId: "team_alpha", workspaceKind: "team", membershipGeneration: 2, supported: false };
        } else throw new Error("Personal data request before team availability");
        return { config, status: 200, statusText: "OK", headers: {}, data: { code: 0, data } };
    };
    const host = document.createElement("div");
    document.body.append(host);
    const root = createRoot(host);
    try {
        await act(async () =>
            root.render(
                <AuthSessionHydrator>
                    <span data-private-workspace>private workspace</span>
                </AuthSessionHydrator>,
            ),
        );
        await flush();
        expect(host.querySelector("[data-private-workspace]")).toBeNull();
        expect(host.querySelector("[role=alert]")?.textContent).toContain("团队空间尚未开放");
        expect(useCreationPreferencesStore.getState().preferences.mode).not.toBe("video");
        expect(reads.filter((key) => key.includes(user.id))).toHaveLength(0);
        expect(getActiveUserScope()).toBe("guest");
        await act(async () => (host.querySelector("button") as HTMLElement).click());
        await flush();
        expect(attempt).toBe(2);
        expect(host.querySelector("[data-private-workspace]")).toBeNull();
    } finally {
        await act(async () => root.unmount());
        host.remove();
        resetRemoteUserDataSync();
    }
});

test("unavailable context stays closed, late previous actor cannot restore a cache, and account uses the pinned context", async () => {
    let release!: (value: unknown) => void;
    let started!: () => void;
    const pending = new Promise<unknown>((resolve) => {
        release = resolve;
    });
    const begun = new Promise<void>((resolve) => {
        started = resolve;
    });
    let mode = "unavailable";
    const bob = { ...user, id: "fixture-bob" };
    apiClient.defaults.adapter = async (config) => {
        let data: unknown;
        if (config.url === "/auth/workspace-context") {
            if (mode === "unavailable") throw new Error("Space is unavailable");
            if (mode === "late") {
                started();
                data = await pending;
            } else data = { ...personal, userId: bob.id, workspaceId: "personal-202" };
        } else if (config.url === "/auth/account-summary") {
            expect(config.params).toEqual({ workspace_id: "personal-202", membership_generation: 0 });
            data = { userId: bob.id, workspaceId: "personal-202", workspaceKind: "personal", membershipGeneration: 0 };
        } else if (config.url === "/model-catalog/summaries") data = { source: "platform", catalogRevision: "catalog-v1", channels: [], hasMore: false };
        else throw new Error("Unexpected request");
        return { config, status: 200, statusText: "OK", headers: {}, data: { code: 0, data } };
    };
    await expect(applyUserSession({ provider: "platform", user })).rejects.toThrow("Space is unavailable");
    expect(useUserStore.getState().hydrated).toBe(false);
    mode = "late";
    const old = applyUserSession({ provider: "platform", user });
    await begun;
    mode = "current";
    await applyUserSession({ provider: "platform", user: bob });
    release(personal);
    await old;
    expect(useUserStore.getState().user?.id).toBe(bob.id);
    expect(useUserStore.getState().workspaceContext?.workspaceId).toBe("personal-202");
    expect(getActiveUserScope()).toBe(bob.id);
    await getStudioAccountSummary(new AbortController().signal);
});
