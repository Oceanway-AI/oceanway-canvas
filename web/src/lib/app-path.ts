export const appBasePath = (import.meta.env.BASE_URL || "/").replace(/\/+$/, "");

/** Browser URLs outside React Router also need the deployment prefix. */
export function appURL(path: string) {
    if (!path.startsWith("/") || path.startsWith("//") || !appBasePath) return path;
    if (path === appBasePath || path.startsWith(`${appBasePath}/`) || path.startsWith(`${appBasePath}?`) || path.startsWith(`${appBasePath}#`)) return path;
    return `${appBasePath}${path}`;
}

/** React Router destinations stay relative to its basename. */
export function appRoute(path: string) {
    if (appBasePath && (path === appBasePath || path.startsWith(`${appBasePath}/`) || path.startsWith(`${appBasePath}?`) || path.startsWith(`${appBasePath}#`))) {
        const route = path.slice(appBasePath.length);
        return route.startsWith("/") ? route : `/${route}`;
    }
    return path;
}
