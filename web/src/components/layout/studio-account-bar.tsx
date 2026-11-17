import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useLocation, useNavigate } from "react-router";
import { Settings, LogOut, HelpCircle } from "lucide-react";
import { OceanAccountAction, OceanAccountPanel, OceanAccountPopover, OceanAccountTrigger } from "@/vendor/oceanway-account-panel-v1/account-panel";
import { OceanWorkspaceSwitcher } from "@/vendor/oceanway-context-switcher-v1/context-switcher";
import { OceanAvatar } from "@/vendor/oceanway-account-v1/account";
import { useStudioAccount } from "@/hooks/use-studio-account";
import { useWorkspaceLogout } from "@/hooks/use-workspace-logout";
import { subscribeGenerationTaskSuccess } from "@/services/generation-completion-tracker";
import { getStudioWorkspaceChoices, studioConsoleNavigationURL } from "@/services/api/studio-account";
import { useUserStore } from "@/stores/use-user-store";
import { getAuthSession, invalidateAuthSessionCache } from "@/services/api/auth";
import { appBasePath, appURL } from "@/lib/app-path";
import { ApiError } from "@/services/api/request";
import { StudioAccountAvatar, StudioTeamAvatar } from "./studio-account-avatar";
import { StudioAccountFinance } from "./studio-account-finance";
import "./studio-shared-controls.css";

export function StudioAccountBar({ compact = false }: { compact?: boolean }) {
    const user = useUserStore((state) => state.user);
    const context = useUserStore((state) => state.workspaceContext);
    const scope = JSON.stringify([user?.id, context?.workspaceId, context?.membershipGeneration]);
    return user ? <StudioAccountControl key={scope} compact={compact} /> : null;
}

function StudioAccountControl({ compact }: { compact: boolean }) {
    const user = useUserStore((state) => state.user)!;
    const context = useUserStore((state) => state.workspaceContext);
    const { pathname } = useLocation();
    const navigate = useNavigate();
    const returnPath = pathname === "/" ? appBasePath || "/" : appURL(pathname);
    const { account, query } = useStudioAccount(returnPath);
    const { handleLogout, loggingOut } = useWorkspaceLogout();
    const [open, setOpen] = useState(false);
    const spaces = useQuery({ queryKey: ["studio-workspace-choices", user.id], queryFn: ({ signal }) => getStudioWorkspaceChoices(signal), enabled: open, retry: false, gcTime: 0, refetchOnWindowFocus: false });
    const { refetch, error } = query;
    useEffect(() => subscribeGenerationTaskSuccess(() => void refetch()), [refetch]);
    useEffect(() => {
        if (!(error instanceof ApiError && error.status === 401)) return;
        let active = true;
        invalidateAuthSessionCache();
        void getAuthSession()
            .then(async (session) => {
                if (!active || session.user?.id === user.id || useUserStore.getState().user?.id !== user.id) return;
                const { applyUserSession } = await import("@/lib/user-session");
                if (active && useUserStore.getState().hydrated && useUserStore.getState().user?.id === user.id) await applyUserSession(session);
            })
            .catch(() => undefined);
        return () => {
            active = false;
        };
    }, [error, user.id]);
    useEffect(() => {
        if (!account) return;
        const current = useUserStore.getState().user;
        if (current?.id === account.userId && (current.displayName !== account.displayName || current.avatarUrl !== account.avatarUrl)) {
            useUserStore.getState().setUser({ ...current, displayName: account.displayName, avatarUrl: account.avatarUrl });
        }
    }, [account]);
    const name = account?.displayName || user.displayName || user.username || "OceanWay 账号";
    const workspaceName = context?.workspaceKind === "team" ? context.workspaceName : context ? "个人空间" : "空间暂不可用";
    const avatar = <StudioAccountAvatar name={name} avatarUrl={account?.avatarUrl || user.avatarUrl} teamName={context?.workspaceKind === "team" ? workspaceName : undefined} />;
    const choices = !spaces.isError && spaces.data?.userId === user.id ? spaces.data.items : [];
    return (
        <OceanAccountPopover open={open} onOpenChange={setOpen} label="账户" trigger={<OceanAccountTrigger name={name} workspace={workspaceName} avatar={avatar} className="ow-studio-account-trigger" data-compact={compact || undefined} />}>
            {open && (
                <OceanAccountPanel
                    name={name}
                    identity={user.username}
                    avatar={avatar}
                    density="compact"
                    tools={
                        <>
                            <OceanWorkspaceSwitcher
                                portalContainer={document.body}
                                current={{
                                    id: context?.workspaceId || "",
                                    label: workspaceName,
                                    icon: context?.workspaceKind === "team" ? <StudioTeamAvatar name={workspaceName} /> : <OceanAvatar displayName={name} avatarUrl={account?.avatarUrl || user.avatarUrl} size={22} />,
                                }}
                                items={choices.map((space) => ({
                                    id: space.id,
                                    label: space.kind === "personal" ? "个人空间" : space.name,
                                    icon: space.kind === "team" ? <StudioTeamAvatar name={space.name} /> : <OceanAvatar displayName={name} avatarUrl={account?.avatarUrl || user.avatarUrl} size={22} />,
                                    disabled: !space.supported || space.id !== context?.workspaceId,
                                    description: !space.supported ? "Studio 团队画布暂未开放" : undefined,
                                }))}
                                label="切换空间"
                                currentLabel="当前空间"
                                loadingLabel="正在读取空间"
                                emptyLabel="暂无可用空间"
                                errorLabel="暂时无法读取空间"
                                retryLabel="重试"
                                pendingLabel="正在切换"
                                state={spaces.isError ? "error" : spaces.isPending ? "loading" : "ready"}
                                onRetry={() => void spaces.refetch()}
                                onSelect={() => {}}
                            />
                            <OceanAccountAction icon={<Settings />} trailing="›" disabled={!account} onClick={() => window.open(studioConsoleNavigationURL(account, "account", returnPath), "_blank", "noopener,noreferrer")}>
                                个人设置
                            </OceanAccountAction>
                            <OceanAccountAction
                                icon={<Settings />}
                                trailing="›"
                                onClick={() => {
                                    setOpen(false);
                                    navigate("/settings?section=profile");
                                }}
                            >
                                Studio 设置
                            </OceanAccountAction>
                            <OceanAccountAction icon={<HelpCircle />} trailing="↗" onClick={() => window.open("https://docs.oceanway.site/docs", "_blank", "noopener,noreferrer")}>
                                文档与帮助
                            </OceanAccountAction>
                            <OceanAccountAction icon={<LogOut />} disabled={loggingOut} onClick={() => void handleLogout()}>
                                {loggingOut ? "正在退出…" : "退出登录"}
                            </OceanAccountAction>
                        </>
                    }
                >
                    <StudioAccountFinance account={account} loading={query.isFetching} retry={() => void refetch()} returnPath={returnPath} />
                    {account && (
                        <a className="ow-shared-account-inline" href={studioConsoleNavigationURL(account, "usage", returnPath)} target="_blank" rel="noopener noreferrer">
                            使用记录 ›
                        </a>
                    )}
                </OceanAccountPanel>
            )}
        </OceanAccountPopover>
    );
}
