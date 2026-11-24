import { apiBaseURL, http } from "./request";
import { useUserStore } from "@/stores/use-user-store";

export type StudioAccountSummary = {
    userId: string;
    displayName: string;
    avatarUrl?: string;
    workspaceId: string;
    workspaceName: string;
    workspaceKind: "personal" | "team";
    membershipGeneration: number;
    membership: { state: string; plan_title?: string; active_count?: number; end_time?: number };
    benefits: { state: string; items?: { id: string; title: string; remaining_count?: string; remaining_points?: string; unlimited: boolean }[] };
    spending: { state: string; available_points?: string; used_points?: string; spending_enabled?: boolean };
    capabilities: { can_read_finance: boolean; can_purchase: boolean };
    balance: { state: string; kind: "personal" | "team" | "member" | "unavailable"; available_points?: string; frozen_points?: string };
    applications: { product: string; url: string }[];
    accountUrl?: string;
    walletUrl?: string;
    usageUrl?: string;
};

export async function getStudioAccountSummary(signal: AbortSignal) {
    const { user, workspaceContext } = useUserStore.getState();
    if (!workspaceContext || workspaceContext.userId !== user?.id) return Promise.reject(new Error("当前空间尚未确认"));
    const data = await http.get<StudioAccountSummary>("/auth/account-summary", { signal, params: { workspace_id: workspaceContext.workspaceId, membership_generation: workspaceContext.membershipGeneration } });
    if (!studioAccountMatches(data, user.id, workspaceContext.workspaceId, workspaceContext.membershipGeneration) || data.workspaceKind !== workspaceContext.workspaceKind) throw new Error("账户或空间已变化，请刷新后重试");
    return data;
}

export function studioAccountMatches(account: StudioAccountSummary | undefined, userId?: string, workspaceId?: string, generation?: number) {
    return !!account && account.userId === userId && account.workspaceId === workspaceId && account.membershipGeneration === generation;
}

export type StudioConsoleDestination = "account" | "models" | "wallet" | "usage" | "subscriptions" | "plans" | "orders" | "recharge";

// The browser opens a fresh BFF request so account changes and page ownership
// are checked before the provider prepares the Console return context.
export function studioConsoleNavigationURL(account: StudioAccountSummary | undefined, destination: StudioConsoleDestination, returnPath: string) {
    if (!account) return undefined;
    const query = new URLSearchParams({ destination, return_path: returnPath, user_id: account.userId, workspace_id: account.workspaceId });
    return `${apiBaseURL.replace(/\/$/, "")}/auth/console-navigation?${query}`;
}

export type StudioWorkspaceChoice = { id: string; name: string; kind: "personal" | "team"; membershipGeneration: number; supported: boolean };
export async function getStudioWorkspaceChoices(signal: AbortSignal) {
    const userId = useUserStore.getState().user?.id;
    const data = await http.get<{ userId: string; items: StudioWorkspaceChoice[] }>("/auth/workspaces", { signal });
    if (!userId || data.userId !== userId || !Array.isArray(data.items)) throw new Error("空间列表已变化");
    return data;
}

export async function getStudioProductNavigation(account: StudioAccountSummary, product: string) {
    const result = await http.get<{ userId: string; workspaceId: string; membershipGeneration: number; url: string }>("/auth/product-navigation", {
        params: { user_id: account.userId, workspace_id: account.workspaceId, membership_generation: account.membershipGeneration, product },
    });
    const current = useUserStore.getState();
    if (
        !studioAccountMatches(account, current.user?.id, current.workspaceContext?.workspaceId, current.workspaceContext?.membershipGeneration) ||
        result.userId !== account.userId ||
        result.workspaceId !== account.workspaceId ||
        result.membershipGeneration !== account.membershipGeneration
    )
        throw new Error("账号或空间已变化");
    const url = new URL(result.url);
    const entry = account.applications.find((app) => app.product === product);
    if (
        !entry ||
        url.protocol !== "https:" ||
        url.username ||
        url.password ||
        url.hash ||
        url.origin !== new URL(entry.url).origin ||
        url.pathname !== new URL(entry.url).pathname ||
        url.searchParams.size !== 2 ||
        url.searchParams.get("workspace_id") !== account.workspaceId ||
        url.searchParams.get("membership_generation") !== String(account.membershipGeneration)
    )
        throw new Error("产品入口已变化，请刷新后重试");
    return url.href;
}

// Media tasks use the Studio task UUID as the grant reference, or the original
// Agent run UUID for an approved step. The provider owns its ledger request ID.
export function studioTaskUsageURL(usageURL: string | undefined, task: { id: string; agentRunId?: string }) {
    if (!usageURL) return undefined;
    const target = new URL(usageURL);
    target.searchParams.set("source", "platform");
    target.searchParams.set("product", "studio");
    target.searchParams.set("business_task_id", task.agentRunId || task.id);
    return target.href;
}
