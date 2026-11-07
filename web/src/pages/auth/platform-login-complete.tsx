import { useEffect, useState } from "react";
import { Link, useNavigate } from "react-router";
import { consumePlatformLoginNext } from "@/lib/platform-login";
import { useUserStore } from "@/stores/use-user-store";
import { FullScreenLoader } from "@/components/ui/aceternity/full-screen-loader";

export default function PlatformLoginComplete() {
    const navigate = useNavigate();
    const user = useUserStore((state) => state.user);
    const hydrated = useUserStore((state) => state.hydrated);
    const [next] = useState(consumePlatformLoginNext);
    useEffect(() => {
        if (hydrated && user) navigate(next, { replace: true });
    }, [hydrated, user, next, navigate]);
    if (!hydrated || user) return <FullScreenLoader />;
    return (
        <section className="auth-panel is-page">
            <h2 className="auth-panel-title">尚未完成登录</h2>
            <p className="auth-panel-description">未能确认 OceanWay 登录状态，请重新登录。</p>
            <Link to="/login">返回登录</Link>
        </section>
    );
}
