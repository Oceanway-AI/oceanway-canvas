import { OceanCreditsCard, OceanMembershipCard } from "@/vendor/oceanway-account-panel-v1/account-panel";
import mark from "@/vendor/oceanway-account-v1/oceanway-ai-app-micro.svg";
import { studioConsoleNavigationURL, type StudioAccountSummary, type StudioConsoleDestination } from "@/services/api/studio-account";

const decimal = (value?: string) => typeof value === "string" && /^-?\d+(\.\d+)?$/.test(value);

export function StudioAccountFinance({ account, loading, retry, returnPath }: { account?: StudioAccountSummary; loading: boolean; retry: () => void; returnPath: string }) {
    function link(destination: StudioConsoleDestination, label: string, className?: string) {
        const href = studioConsoleNavigationURL(account, destination, returnPath);
        return href ? (
            <button type="button" onClick={() => window.open(href, "_blank", "noopener,noreferrer")} className={className}>
                {label}
            </button>
        ) : undefined;
    }
    const membership = account?.membership;
    const title = membership?.state === "ready" ? membership.plan_title || (membership.active_count === 0 ? "暂无有效订阅" : "有效订阅") : membership?.state === "restricted" ? "团队权益" : loading ? "正在读取订阅" : "暂时无法读取订阅";
    const balance = account?.balance;
    const ready = balance?.state === "ready" && decimal(balance.available_points);
    const canPurchase = account?.capabilities?.can_purchase === true;
    const partialFailure = [membership?.state, account?.benefits?.state, account?.spending?.state].includes("unavailable");
    return (
        <>
            <OceanMembershipCard
                mark={<img src={mark} alt="" />}
                title={title}
                scope={account?.workspaceKind === "team" ? account.workspaceName : "个人空间"}
                action={canPurchase ? link("plans", "查看方案 ›", "ow-shared-account-pill") : undefined}
                links={
                    account ? (
                        <>
                            {link("subscriptions", "订阅与权益")}
                            {account.capabilities?.can_read_finance && link("orders", "付款订单")}
                        </>
                    ) : undefined
                }
            />
            <OceanCreditsCard label={balance?.kind === "member" ? "本人可用积分" : "当前空间积分"} amount={ready ? `✦ ${balance.available_points}` : "—"} details={link("wallet", "查看积分 ›", "ow-shared-account-inline")}>
                {!ready && <span role="status">{loading ? "正在读取积分" : balance?.state === "restricted" ? "暂无查看权限" : "暂时无法读取积分"}</span>}
                {ready && decimal(balance.frozen_points) && (
                    <div className="ow-shared-account-metric">
                        <span>待结算</span>
                        <span>✦ {balance.frozen_points}</span>
                    </div>
                )}
                {account?.spending?.state === "ready" && decimal(account.spending.used_points) && (
                    <div className="ow-shared-account-metric">
                        <span>本人本期已用</span>
                        <span>✦ {account.spending.used_points}</span>
                    </div>
                )}
                {account?.spending?.state === "unavailable" && <span role="status">暂时无法读取本人用量</span>}
                {canPurchase && link("recharge", "充值 ›", "ow-shared-account-inline")}
            </OceanCreditsCard>
            {account?.benefits?.state === "unavailable" && <span role="status">暂时无法读取可用权益</span>}
            {account?.benefits?.state === "ready" && (
                <details className="ow-shared-account-benefits">
                    <summary>可用权益</summary>
                    {!account.benefits.items?.length && <span>暂无可用权益</span>}
                    {(account.benefits.items ?? []).map((item) => (
                        <div key={item.id} className="ow-shared-account-metric">
                            <span>{item.title}</span>
                            <span>{item.unlimited ? "不限量" : decimal(item.remaining_count) ? `${item.remaining_count} 次` : decimal(item.remaining_points) ? `✦ ${item.remaining_points}` : "查看详情"}</span>
                        </div>
                    ))}
                </details>
            )}
            {(!account || !ready || partialFailure) && (
                <button type="button" className="ow-shared-account-inline" disabled={loading} onClick={retry}>
                    重新读取
                </button>
            )}
        </>
    );
}
