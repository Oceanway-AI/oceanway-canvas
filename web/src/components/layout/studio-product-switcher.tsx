import { useEffect, useRef, useState } from "react";
import { OceanProductSwitcher } from "@/vendor/oceanway-context-switcher-v1/context-switcher";
import mark from "@/vendor/oceanway-account-v1/oceanway-ai-app-micro.svg";
import { useStudioAccount } from "@/hooks/use-studio-account";
import { getStudioProductNavigation } from "@/services/api/studio-account";
import { flushCanvasStorePersistence } from "@/stores/canvas/use-canvas-store";
import { flushAssetStorePersistence } from "@/stores/use-asset-store";
import "./studio-shared-controls.css";

const names: Record<string, string> = { studio: "Studio", console: "Console", drama: "Drama", commerce: "Commerce" };

export function StudioProductSwitcher({ compact = false, navigate = (url: string) => window.location.assign(url) }: { compact?: boolean; navigate?: (url: string) => void }) {
    const { account, query } = useStudioAccount();
    const [pending, setPending] = useState<string>();
    const [failed, setFailed] = useState(false);
    const attempt = useRef(0);
    const scope = JSON.stringify([account?.userId, account?.workspaceId, account?.membershipGeneration]);
    const latest = useRef(scope);
    latest.current = scope;
    useEffect(() => {
        attempt.current++;
        setPending(undefined);
        setFailed(false);
    }, [scope]);
    useEffect(
        () => () => {
            attempt.current++;
        },
        [],
    );
    async function select(product: string) {
        if (!account || pending || product === "studio") return;
        const current = ++attempt.current;
        setPending(product);
        setFailed(false);
        try {
            await Promise.all([flushCanvasStorePersistence(), flushAssetStorePersistence()]);
            if (current !== attempt.current || latest.current !== scope) return;
            const url = await getStudioProductNavigation(account, product);
            if (current === attempt.current && latest.current === scope) navigate(url);
        } catch {
            if (current === attempt.current && latest.current === scope) setFailed(true);
        } finally {
            if (current === attempt.current && latest.current === scope) setPending(undefined);
        }
    }
    return (
        <OceanProductSwitcher
            portalContainer={document.body}
            compact={compact}
            className="ow-studio-product-switcher"
            current={{ id: "studio", label: "Studio", icon: <img src={mark} alt="" /> }}
            items={(account?.applications ?? []).map((app) => ({ id: app.product, label: names[app.product] || app.product, icon: <img src={mark} alt="" /> }))}
            label="切换 OceanWay 产品"
            currentLabel="当前产品"
            loadingLabel="正在读取产品"
            emptyLabel="暂无可用产品"
            errorLabel="暂时无法读取产品"
            retryLabel="重试"
            pendingLabel="正在打开"
            state={query.isError ? "error" : !account ? "loading" : "ready"}
            error={failed ? "暂时无法打开，请重试" : undefined}
            pendingId={pending}
            onSelect={(id) => void select(id)}
            onRetry={() => void query.refetch()}
        />
    );
}
