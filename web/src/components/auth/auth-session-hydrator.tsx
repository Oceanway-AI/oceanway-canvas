import type { ReactNode } from "react";
import { Fragment, useEffect, useState } from "react";
import { Button } from "antd";

import { getAuthSession, invalidateAuthSessionCache, type AuthSessionPayload } from "@/services/api/auth";
import { ApiError } from "@/services/api/request";
import { ACTIVE_USER_SCOPE_KEY } from "@/lib/user-scope";
import { appQueryClient } from "@/lib/query-client";
import { appRoute } from "@/lib/app-path";
import { FullScreenLoader } from "@/components/ui/aceternity/full-screen-loader";
import { preloadWorkspaceRoute } from "@/lib/workspace-route-modules";
import { useUserStore } from "@/stores/use-user-store";
import { recordDiagnosticEvent } from "@/services/diagnostics/client-diagnostics";

export function AuthSessionHydrator({ children }: { children: ReactNode }) {
    const hydrated = useUserStore((state) => state.hydrated);
    const workspaceError = useUserStore((state) => state.workspaceError);
    const [attempt, setAttempt] = useState(0);
    const [sessionError, setSessionError] = useState(false);
    const [startupReady, setStartupReady] = useState(false);

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
        setSessionError(false);
        setStartupReady(false);
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
                    if (!cancelled) setStartupReady(true);
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
                if (!cancelled) setStartupReady(true);
                recordDiagnosticEvent({
                    category: "navigation",
                    level: "info",
                    code: "startup.workspace_ready",
                    message: "登录工作区已解除启动阻塞",
                    durationMs: performance.now() - startedAt,
                });
                preloadWorkspaceRoute(appRoute(window.location.pathname));
            })
            .catch(async (error) => {
                if (!cancelled) {
                    // Failed workspace verification is not an anonymous session
                    // and must not open children with guest or personal state.
                    if (useUserStore.getState().workspaceError) return;
                    if (error instanceof ApiError && error.status === 401) {
                        await applyAnonymousSession({ user: null });
                        if (!cancelled) setStartupReady(true);
                        return;
                    }
                    setSessionError(true);
                    recordDiagnosticEvent({
                        category: "navigation",
                        level: "warning",
                        code: "startup.auth_session_failed",
                        message: "认证会话恢复失败，工作区保持关闭",
                        durationMs: performance.now() - startedAt,
                    });
                }
            });
        return () => {
            cancelled = true;
        };
    }, [attempt]);

    if (workspaceError || sessionError) {
        return (
            <main className="flex min-h-dvh items-center justify-center bg-background p-6">
                <div className="max-w-sm space-y-4 text-center">
                    <p role="alert" className="text-sm text-muted-foreground">
                        {workspaceError || "暂时无法确认登录状态，请重试。"}
                    </p>
                    <Button
                        onClick={() => {
                            invalidateAuthSessionCache();
                            useUserStore.setState({ workspaceError: null, hydrated: false });
                            setAttempt((value) => value + 1);
                        }}
                    >
                        重试
                    </Button>
                </div>
            </main>
        );
    }

    return startupReady && hydrated ? <Fragment key={userId ?? "guest"}>{children}</Fragment> : <FullScreenLoader />;
}

async function applyAnonymousSession(payload: AuthSessionPayload) {
    const { applyUserSession } = await import("@/lib/user-session");
    await applyUserSession({ ...payload, user: null });
}
