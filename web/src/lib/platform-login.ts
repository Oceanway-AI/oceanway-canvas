const nextKey = "oceanway:studio:login-next";

export function platformLoginNext(value?: string | null) {
    if (!value?.startsWith("/") || value.startsWith("//") || /[\\\u0000-\u0020]/.test(value)) return "/";
    const url = new URL(value, "https://studio.invalid");
    const path = url.pathname.replace(/\/+$/, "").toLowerCase();
    if (url.pathname.startsWith("//") || ["/platform/login/complete", "/login", "/register", "/forgot-password"].includes(path)) return "/";
    return url.pathname + url.search + url.hash;
}

export function platformLoginPage(value: string) {
    const next = platformLoginNext(value);
    return next === "/" ? "/login" : "/login?next=" + encodeURIComponent(next);
}

export function rememberPlatformLoginNext(value: string) {
    try {
        window.sessionStorage.setItem(nextKey, platformLoginNext(value));
    } catch {
        /* The fixed callback also works without storage. */
    }
}

export function readPlatformLoginNext() {
    try {
        return platformLoginNext(window.sessionStorage.getItem(nextKey));
    } catch {
        return "/";
    }
}

export function clearPlatformLoginNext() {
    try {
        window.sessionStorage.removeItem(nextKey);
    } catch {
        /* Storage can be disabled. */
    }
}
