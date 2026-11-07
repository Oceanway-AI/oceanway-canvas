import { useId } from "react";
import type { AuthView } from "@/lib/auth-entry";
import { cn } from "@/lib/utils";
import { PlatformLoginPanel } from "./platform-login-panel";
import "./auth-panel.css";

export type AuthPanelProps = {
    view: AuthView;
    onViewChange: (view: AuthView) => void;
    next: string;
    onAuthenticated: () => void;
    /** page：独立登录页里的卡片；dialog：登录弹窗右侧，由弹窗提供外壳。 */
    variant: "page" | "dialog";
};

/** Studio always sends account authentication to OceanWay. */
export function AuthPanel({ next, variant }: AuthPanelProps) {
    const titleId = useId();

    return (
        <section className={cn("auth-panel", variant === "page" ? "is-page" : "is-dialog")} aria-labelledby={titleId}>
            <header className="auth-panel-header">
                <h2 id={titleId} className="auth-panel-title">
                    登录 OceanWay Studio
                </h2>
            </header>
            <div className="auth-panel-body">
                <PlatformLoginPanel next={next} />
            </div>
        </section>
    );
}
