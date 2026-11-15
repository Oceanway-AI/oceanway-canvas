import { useEffect } from "react";
import { Link } from "react-router";
import { OceanAccountBar } from "@/vendor/oceanway-account-v1/account";
import { useStudioAccount } from "@/hooks/use-studio-account";
import { useWorkspaceLogout } from "@/hooks/use-workspace-logout";
import { subscribeGenerationTaskSuccess } from "@/services/generation-completion-tracker";
import { useUserStore } from "@/stores/use-user-store";
import { getAuthSession, invalidateAuthSessionCache } from "@/services/api/auth";
import { ApiError } from "@/services/api/request";

export function StudioAccountBar({ compact = false }: { compact?: boolean }) {
    const userId = useUserStore((state) => state.user?.id);
    const { account, query } = useStudioAccount();
    const { handleLogout, loggingOut } = useWorkspaceLogout();
    const { refetch, data, error } = query;
    useEffect(() => subscribeGenerationTaskSuccess(() => void refetch()), [refetch]);
    useEffect(() => {
        if (!userId || (!(data && data.userId !== userId) && !(error instanceof ApiError && error.status === 401))) return;
        let active = true;
        invalidateAuthSessionCache();
        void getAuthSession()
            .then(async (session) => {
                if (!active || session.user?.id === userId || useUserStore.getState().user?.id !== userId) return;
                const { applyUserSession } = await import("@/lib/user-session");
                if (active && useUserStore.getState().hydrated && useUserStore.getState().user?.id === userId) await applyUserSession(session);
            })
            .catch(() => undefined);
        return () => {
            active = false;
        };
    }, [data, error, userId]);
    useEffect(() => {
        if (!account) return;
        const current = useUserStore.getState().user;
        if (current?.id !== account.userId) return;
        if (current.displayName !== account.displayName || current.avatarUrl !== account.avatarUrl) {
            useUserStore.getState().setUser({ ...current, displayName: account.displayName, avatarUrl: account.avatarUrl });
        }
    }, [account]);
    if (!userId) return null;
    const amount = account?.balance?.available_points;
    const spaceName = account?.workspaceName && account.workspaceName !== "Personal space" ? account.workspaceName : "个人空间";
    const ready = account?.balance?.kind === "personal" && typeof amount === "string" && /^-?\d+(\.\d+)?$/.test(amount);
    return (
        <OceanAccountBar
            productName="Studio"
            products={(account?.applications ?? []).map((app) => ({ name: app.product.toLowerCase() === "studio" ? "Studio" : app.product.toLowerCase() === "console" ? "Console" : app.product, href: app.url }))}
            workspaceId={account?.workspaceId}
            workspaces={account ? [{ id: account.workspaceId, name: spaceName }] : []}
            displayName={account?.displayName || "OceanWay 账号"}
            avatarUrl={account?.avatarUrl}
            balance={ready ? { state: "ready", amount } : { state: query.isPending ? "loading" : error instanceof ApiError && error.status === 403 ? "restricted" : "unavailable" }}
            balanceLabel="OceanWay 通用余额"
            accountHref={account?.accountUrl}
            walletHref={account?.walletUrl}
            linkTarget="_blank"
            onRetry={() => void refetch()}
            compact={compact}
            accountDetails={
                <>
                    {account?.usageUrl && (
                        <a href={account.usageUrl} target="_blank" rel="noopener noreferrer">
                            使用记录
                        </a>
                    )}
                    <Link to="/settings?section=profile">Studio 设置</Link>
                    <button type="button" disabled={loggingOut} onClick={() => void handleLogout()}>
                        退出登录
                    </button>
                </>
            }
        />
    );
}
