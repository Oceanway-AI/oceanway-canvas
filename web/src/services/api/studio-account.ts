import { http } from "./request";

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
    return http.get<StudioAccountSummary>("/auth/account-summary", { signal });
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
