import { useQuery } from "@tanstack/react-query";
import { Button } from "antd";
import { getPlatformMaterialStorage } from "@/services/api/platform-materials";
import { useUserStore } from "@/stores/use-user-store";

function bytes(value: string) {
    if (!/^\d+$/.test(value)) return "暂不可用";
    const n = BigInt(value),
        unit = BigInt(1024 * 1024 * 1024);
    return `${n / unit}.${(((n % unit) * BigInt(100)) / unit).toString().padStart(2, "0")} GB`;
}
export function PlatformStoragePane({ compact = false }: { compact?: boolean }) {
    const userId = useUserStore((state) => state.user?.id);
    const query = useQuery({ queryKey: ["platform-storage", userId], queryFn: ({ signal }) => getPlatformMaterialStorage(signal), enabled: !!userId, retry: false });
    return (
        <section className="space-y-3" aria-label="平台素材空间">
            {!compact ? (
                <div className="settings-pane-header">
                    <div>
                        <h2>素材空间</h2>
                        <p>所有素材统一保存在 OceanWay 账号中。</p>
                    </div>
                </div>
            ) : null}
            {!userId ? (
                <p>请先登录。</p>
            ) : query.isPending ? (
                <p role="status">正在读取空间…</p>
            ) : query.isError ? (
                <div role="alert">
                    空间读取失败。<Button onClick={() => void query.refetch()}>重试</Button>
                </div>
            ) : query.data ? (
                <>
                    <p>
                        已用 {bytes(query.data.used_bytes)} / {bytes(query.data.limit_bytes)}
                    </p>
                    <p>
                        上传预留 {bytes(query.data.reserved_bytes)} · 剩余 {bytes(query.data.remaining_bytes)}
                    </p>
                    {query.data.over_limit ? <p role="alert">素材空间已满，请在账户中心管理容量。</p> : null}
                </>
            ) : null}
        </section>
    );
}
