import { useQuery } from "@tanstack/react-query";
import { Button, Skeleton } from "antd";
import { loadProfileSession } from "@/services/profile-session";
import { useUserStore } from "@/stores/use-user-store";
import "./profile-pane.css";

export function ProfilePane() {
    const userId = useUserStore((state) => state.user?.id);
    const profileQuery = useQuery({
        queryKey: ["user-profile", userId],
        queryFn: ({ signal }) => loadProfileSession(userId, signal),
        enabled: !!userId,
        retry: false,
        refetchOnWindowFocus: false,
        staleTime: 0,
    });
    const profile = profileQuery.data;

    return (
        <div className="profile-pane">
            <div className="settings-pane-header">
                <div>
                    <h2>OceanWay 账号</h2>
                    <p>账号资料和安全设置在账户中心管理。</p>
                </div>
            </div>
            {!userId ? (
                <p>请先登录 OceanWay 账号。</p>
            ) : profileQuery.isPending ? (
                <Skeleton active paragraph={{ rows: 2 }} />
            ) : profileQuery.isError || profile?.provider !== "platform" || profile.user.id !== userId ? (
                <div>
                    <p role="alert">暂时无法读取账号资料，请重试。</p>
                    <Button loading={profileQuery.isFetching} onClick={() => void profileQuery.refetch()}>
                        重试
                    </Button>
                </div>
            ) : (
                <div>
                    <p>{profile.user.displayName || profile.user.username}</p>
                    {profile.user.email ? <p>{profile.user.email}</p> : null}
                    {profile.consoleURL ? (
                        <Button href={profile.consoleURL} target="_blank" rel="noopener noreferrer">
                            打开账户中心
                        </Button>
                    ) : null}
                </div>
            )}
        </div>
    );
}
