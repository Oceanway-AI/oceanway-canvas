import { safeAuthNext } from "@/lib/auth-entry";

const nextKey = "oceanway:studio:login-next";

export function platformLoginNext(value?: string | null) {
    const next = safeAuthNext(value);
    if (/[\\\u0000-\u0020]/.test(next)) return "/";
    const path = next.split(/[?#]/)[0];
    if (["/platform/login/complete", "/login", "/register", "/forgot-password"].includes(path)) return "/";
    return next;
}

export function rememberPlatformLoginNext(value: string) {
    try {
        window.sessionStorage.setItem(nextKey, platformLoginNext(value));
    } catch {
        /* Navigation still works without browser storage. */
    }
}

export function consumePlatformLoginNext() {
    try {
        const value = window.sessionStorage.getItem(nextKey);
        window.sessionStorage.removeItem(nextKey);
        return platformLoginNext(value);
    } catch {
        return "/";
    }
}
