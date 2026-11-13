import { useEffect, useEffectEvent, useRef, useState } from "react";
import { Button } from "antd";
import { AppModal } from "@/components/ui/product/app-modal";
import { rememberPlatformLoginNext } from "@/lib/platform-login";
import { getAuthSession, getPlatformLoginURL, invalidateAuthSessionCache } from "@/services/api/auth";
import { useAuthDialogStore } from "@/stores/use-auth-dialog-store";

type Props = { next: string; recovery?: boolean; onDismiss: () => void };

const statusText = {
    loading: "正在连接 OceanWay…",
    verifying: "正在确认登录状态…",
    unconfirmed: "未能确认 OceanWay 登录状态。",
    unavailable: "暂时无法载入 OceanWay 登录表单。",
};

export default function PlatformLoginPanel({ next, recovery = false, onDismiss }: Props) {
    const [attempt, setAttempt] = useState(0);
    const [frameURL, setFrameURL] = useState("");
    const [standaloneURL, setStandaloneURL] = useState("");
    const [phase, setPhase] = useState<"loading" | "ready" | "unavailable" | "verifying" | "unconfirmed">("loading");
    const [height, setHeight] = useState(570);
    const frame = useRef<HTMLIFrameElement>(null);
    const dismiss = useEffectEvent(onDismiss);

    useEffect(() => {
        const controller = new AbortController();
        const active = () => !controller.signal.aborted;
        let origin = "";
        let verifying = false;
        const timer = window.setTimeout(() => {
            controller.abort();
            setPhase("unavailable");
        }, 12000);
        setPhase("loading");
        setFrameURL("");
        setStandaloneURL("");

        async function prepare() {
            try {
                const { url } = await getPlatformLoginURL(controller.signal);
                if (!active()) return;
                const target = new URL(url);
                if (target.protocol !== "https:" || target.username || target.password || target.pathname !== "/login" || target.searchParams.get("return") !== window.location.origin + "/platform/login/complete") throw new Error();
                origin = target.origin;
                if (recovery) target.searchParams.set("auth", "forgot");
                setStandaloneURL(target.href);
                target.searchParams.set("embed", "product");
                setFrameURL(target.href);
            } catch {
                window.clearTimeout(timer);
                if (active()) setPhase("unavailable");
            }
        }

        async function verify() {
            verifying = true;
            window.clearTimeout(timer);
            setPhase("verifying");
            try {
                invalidateAuthSessionCache();
                const session = await getAuthSession();
                if (!active()) return;
                if (session.provider !== "platform" || !session.user?.id) throw new Error();
                const { applyUserSession } = await import("@/lib/user-session");
                if (!active()) return;
                useAuthDialogStore.getState().closeDialog();
                await applyUserSession(session);
            } catch {
                if (active()) setPhase("unconfirmed");
            }
        }

        function receive(event: MessageEvent) {
            if (!active() || !origin || !frame.current?.contentWindow || event.source !== frame.current.contentWindow || event.origin !== origin || verifying) return;
            switch (event.data?.type) {
                case "oceanway:login-ready":
                    window.clearTimeout(timer);
                    setPhase("ready");
                    break;
                case "oceanway:login-state":
                    if (Number.isFinite(event.data.height) && event.data.height > 0) setHeight(Math.min(1200, Math.max(320, event.data.height)));
                    break;
                case "oceanway:login-dismiss":
                    dismiss();
                    break;
                case "oceanway:login-complete":
                    void verify();
                    break;
            }
        }

        window.addEventListener("message", receive);
        void prepare();
        return () => {
            controller.abort();
            window.clearTimeout(timer);
            window.removeEventListener("message", receive);
        };
    }, [attempt, recovery]);

    const failed = phase === "unavailable" || phase === "unconfirmed";
    return (
        <AppModal open centered flush footer={null} width={940} onCancel={onDismiss} title={<span className="sr-only">OceanWay 统一登录</span>} styles={{ header: { margin: 0, padding: 0 } }}>
            <div className="grid md:grid-cols-2">
                <div className="relative hidden md:block">
                    <img className="absolute inset-0 size-full object-cover" src="/inspiration-thumbs/haohaoxue/jellyfish-neon-ocean.webp" alt="" />
                </div>
                <div className="relative min-w-0 overflow-hidden bg-background" style={{ height: `min(${height}px, calc(100dvh - 64px))` }}>
                    {frameURL && !failed && (
                        <iframe ref={frame} src={frameURL} title="OceanWay 统一登录表单" allow="publickey-credentials-get" referrerPolicy="no-referrer" tabIndex={phase === "ready" ? 0 : -1} inert={phase !== "ready"} className="block size-full border-0" />
                    )}
                    {phase !== "ready" && (
                        <div className="absolute inset-0 flex flex-col items-center justify-center gap-4 bg-background p-6 text-center">
                            <p role={failed ? "alert" : "status"}>{statusText[phase]}</p>
                            {failed && <Button onClick={() => setAttempt((value) => value + 1)}>重试</Button>}
                            {phase === "unavailable" && standaloneURL && (
                                <a href={standaloneURL} onClick={() => rememberPlatformLoginNext(next)}>
                                    前往 OceanWay 登录
                                </a>
                            )}
                        </div>
                    )}
                </div>
            </div>
        </AppModal>
    );
}
