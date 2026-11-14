import type { ReactNode } from "react";
import { Fragment, useEffect } from "react";

import { getAuthSession, invalidateAuthSessionCache, type AuthSessionPayload } from "@/services/api/auth";
import { ApiError } from "@/services/api/request";
import { ACTIVE_USER_SCOPE_KEY } from "@/lib/user-scope";
import { appQueryClient } from "@/lib/query-client";
import { FullScreenLoader } from "@/components/ui/aceternity/full-screen-loader";
import { preloadWorkspaceRoute } from "@/lib/workspace-route-modules";
import { useUserStore } from "@/stores/use-user-store";
import { recordDiagnosticEvent } from "@/services/diagnostics/client-diagnostics";

export function AuthSessionHydrator({ children }: { children: ReactNode }) {
    const hydrated = useUserStore((state) => state.hydrated);

    const userId = useUserStore((state) => state.user?.id);
    const platformIdentity = useUserStore((state) => state.platformIdentity);
    useEffect(() => {
        if (!platformIdentity) return;
        let version = 0;
        let scheduled = false;
        let active = true;
        const check = async (requestVersion: number) => {
            if (!active || !useUserStore.getState().hydrated) return;
            invalidateAuthSessionCache();
            try {
                const session = await getAuthSession().catch((error) => {
                    if (error instanceof ApiError && error.status === 401) return { provider: "platform", user: null } as const;
                    throw error;
                });
                const current = useUserStore.getState();
                if (!active || requestVersion !== version || !current.hydrated || current.user?.id !== userId) return;
                if (session.user?.id !== userId) {
                    const { applyUserSession } = await import("@/lib/user-session");
                    if (active && requestVersion === version && useUserStore.getState().user?.id === userId) await applyUserSession(session);
                } else if (userId) {
                    await appQueryClient.invalidateQueries({ queryKey: ["studio-account-summary", userId] });
                }
            } catch {
                // A temporary transport failure does not establish a new identity.
            }
        };
        const refresh = () => {
            if (document.visibilityState === "hidden") return;
            version++;
            if (scheduled) return;
            scheduled = true;
            queueMicrotask(() => {
                scheduled = false;
                void check(version);
            });
        };
        const storage = (event: StorageEvent) => {
            if (event.key === ACTIVE_USER_SCOPE_KEY || event.key === null) refresh();
        };
        window.addEventListener("focus", refresh);
        window.addEventListener("pageshow", refresh);
        document.addEventListener("visibilitychange", refresh);
        window.addEventListener("storage", storage);
        return () => {
            active = false;
            window.removeEventListener("focus", refresh);
            window.removeEventListener("pageshow", refresh);
            document.removeEventListener("visibilitychange", refresh);
            window.removeEventListener("storage", storage);
        };
    }, [platformIdentity, userId]);

    useEffect(() => {
        let cancelled = false;
        const startedAt = performance.now();
        recordDiagnosticEvent({ category: "navigation", level: "info", code: "startup.auth_session_started", message: "开始恢复认证会话" });
        getAuthSession()
            .then(async (payload) => {
                if (cancelled) return;
                recordDiagnosticEvent({
                    category: "navigation",
                    level: "info",
                    code: "startup.auth_session_ready",
                    message: payload.user ? "认证会话已恢复" : "匿名会话已确认",
                    durationMs: performance.now() - startedAt,
                });
                if (!payload.user) {
                    await applyAnonymousSession(payload);
                    recordDiagnosticEvent({
                        category: "navigation",
                        level: "info",
                        code: "startup.anonymous_ready",
                        message: "匿名页面已解除启动阻塞",
                        durationMs: performance.now() - startedAt,
                    });
                    return;
                }
                // 账号数据、画布和素材持久化只属于已登录工作区，登录页不下载这些模块。
                const { applyUserSession } = await import("@/lib/user-session");
                if (cancelled) return;
                await applyUserSession(payload);
                recordDiagnosticEvent({
                    category: "navigation",
                    level: "info",
                    code: "startup.workspace_ready",
                    message: "登录工作区已解除启动阻塞",
                    durationMs: performance.now() - startedAt,
                });
                preloadWorkspaceRoute(window.location.pathname);
            })
            .catch(async () => {
                if (!cancelled) {
                    await applyAnonymousSession({ user: null });
                    recordDiagnosticEvent({
                        category: "navigation",
                        level: "warning",
                        code: "startup.auth_session_failed",
                        message: "认证会话恢复失败，已降级为匿名页面",
                        durationMs: performance.now() - startedAt,
                    });
                }
            });
        return () => {
            cancelled = true;
        };
    }, []);

    return hydrated ? <Fragment key={userId ?? "guest"}>{children}</Fragment> : <FullScreenLoader />;
}

async function applyAnonymousSession(payload: AuthSessionPayload) {
    const { applyUserSession } = await import("@/lib/user-session");
    await applyUserSession({ ...payload, user: null });
}
