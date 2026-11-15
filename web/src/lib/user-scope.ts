export const ACTIVE_USER_SCOPE_KEY = "yingce:active-user-scope";
const GUEST_SCOPE = "guest";
// Only this tab’s authenticated session may switch its persistence namespace.
let tabUserScope: string | undefined;
let userScopedPersistenceSuppressionDepth = 0;

export function withUserScopedPersistenceSuppressed<T>(operation: () => T) {
    userScopedPersistenceSuppressionDepth += 1;
    try {
        return operation();
    } finally {
        userScopedPersistenceSuppressionDepth -= 1;
    }
}

export function isUserScopedPersistenceSuppressed() {
    return userScopedPersistenceSuppressionDepth > 0;
}

export function getActiveUserScope() {
    if (typeof window === "undefined") return GUEST_SCOPE;
    return tabUserScope ?? GUEST_SCOPE;
}

export function setActiveUserScope(userId?: string | null) {
    if (typeof window === "undefined") return;
    tabUserScope = userId || GUEST_SCOPE;
    window.localStorage.setItem(ACTIVE_USER_SCOPE_KEY, tabUserScope);
}

export function scopedStorageKey(name: string, scope = getActiveUserScope()) {
    return `${name}:user:${scope}`;
}

export const scopedLocalStorage = {
    getItem: (name: string) => {
        if (typeof window === "undefined") return null;
        return window.localStorage.getItem(scopedStorageKey(name));
    },
    setItem: (name: string, value: string) => {
        if (typeof window === "undefined") return;
        if (isUserScopedPersistenceSuppressed()) return;
        window.localStorage.setItem(scopedStorageKey(name), value);
    },
    removeItem: (name: string) => {
        if (typeof window === "undefined") return;
        if (isUserScopedPersistenceSuppressed()) return;
        window.localStorage.removeItem(scopedStorageKey(name));
    },
};
