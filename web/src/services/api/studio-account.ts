import { apiBaseURL, http } from "./request";
import { useUserStore } from "@/stores/use-user-store";

export type StudioAccountSummary = {
    userId: string;
    displayName: string;
    avatarUrl?: string;
    workspaceId: string;
    workspaceName: string;
    balance: { kind: "personal" | "team" | "member" | "unavailable"; available_points?: string };
    applications: { product: string; url: string }[];
    accountUrl?: string;
    walletUrl?: string;
    usageUrl?: string;
};

export function getStudioAccountSummary(signal: AbortSignal) {
    const { user, workspaceContext } = useUserStore.getState();
    if (!workspaceContext || workspaceContext.userId !== user?.id) return Promise.reject(new Error("当前空间尚未确认"));
    return http.get<StudioAccountSummary>("/auth/account-summary", { signal, params: { workspace_id: workspaceContext.workspaceId, membership_generation: workspaceContext.membershipGeneration } });
}

// The browser opens a fresh BFF request so account changes and page ownership
// are checked before the provider prepares the Console return context.
export function studioConsoleNavigationURL(account: StudioAccountSummary | undefined, destination: "account" | "wallet" | "usage", returnPath: string) {
    if (!account) return undefined;
    const query = new URLSearchParams({ destination, return_path: returnPath, user_id: account.userId, workspace_id: account.workspaceId });
    return `${apiBaseURL.replace(/\/$/, "")}/auth/console-navigation?${query}`;
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
