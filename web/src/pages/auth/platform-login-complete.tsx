import { useEffect, useState } from "react";
import { Link, Navigate } from "react-router";
import { clearPlatformLoginNext, platformLoginPage, readPlatformLoginNext } from "@/lib/platform-login";
import { useUserStore } from "@/stores/use-user-store";

export default function PlatformLoginComplete() {
    const user = useUserStore((state) => state.user);
    const [next] = useState(readPlatformLoginNext);
    useEffect(clearPlatformLoginNext, []);
    if (user) return <Navigate to={next} replace />;
    return (
        <main className="grid min-h-dvh place-content-center gap-3 p-6">
            <h1>尚未完成登录</h1>
            <p>未能确认 OceanWay 登录状态，请重新登录。</p>
            <Link to={platformLoginPage(next)}>返回登录</Link>
        </main>
    );
}
