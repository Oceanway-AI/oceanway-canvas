import { useState } from "react";
import { Button } from "antd";
import { getPlatformLoginURL } from "@/services/api/auth";
import { rememberPlatformLoginNext } from "@/lib/platform-login";

export function PlatformLoginPanel({ next }: { next: string }) {
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState("");
    async function openLogin() {
        setBusy(true);
        setError("");
        try {
            const result = await getPlatformLoginURL();
            const target = new URL(result.url);
            if (target.protocol !== "https:" || target.username || target.password) throw new Error("登录地址不可用");
            rememberPlatformLoginNext(next);
            window.location.assign(target.href);
        } catch {
            setError("暂时无法登录，请稍后重试。");
            setBusy(false);
        }
    }
    return (
        <div className="flex flex-col gap-4">
            <p className="auth-panel-description">使用 OceanWay 账号登录 Studio。</p>
            <Button type="primary" block loading={busy} onClick={() => void openLogin()}>
                登录 OceanWay
            </Button>
            {error ? <p role="alert">{error}</p> : null}
        </div>
    );
}
