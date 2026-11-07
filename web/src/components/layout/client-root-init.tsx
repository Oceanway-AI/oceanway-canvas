import type { ReactNode } from "react";
import { useEffect, useRef } from "react";
import { App } from "antd";
import { initializeClientDiagnostics, setDiagnosticUserScope } from "@/services/diagnostics/client-diagnostics";
import { usePluginStore } from "@/stores/use-plugin-store";
import { useUserStore } from "@/stores/use-user-store";

export function ClientRootInit({ children }: { children: ReactNode }) {
    const userId = useUserStore((state) => state.user?.id || "");
    const { message } = App.useApp();
    const handledConfigParams = useRef(false);
    const setRuntimeStatuses = usePluginStore((state) => state.setRuntimeStatuses);
    const setPluginStates = usePluginStore((state) => state.setPluginStates);
    const pluginStoreHydrated = usePluginStore((state) => state.hydrated);
    const installations = usePluginStore((state) => state.installations);

    useEffect(
        () => () => {
            usePluginStore.getState().setRuntimeStatuses({});
            usePluginStore.getState().setPluginStates({});
        },
        [],
    );

    useEffect(() => {
        if (!pluginStoreHydrated) return;
        setRuntimeStatuses(Object.fromEntries(installations.map(({ manifest }) => [manifest.id, "disabled"])));
        setPluginStates(
            Object.fromEntries(
                installations.map(({ manifest }) => [
                    manifest.id,
                    {
                        pluginId: manifest.id,
                        platformAvailable: false,
                        userEnabled: false,
                        userConfigured: false,
                        effectiveEnabled: false,
                        canToggle: false,
                        canConfigure: false,
                        blockedReason: "此功能暂不可用",
                    },
                ]),
            ),
        );
    }, [installations, pluginStoreHydrated, setPluginStates, setRuntimeStatuses, userId]);

    useEffect(() => {
        initializeClientDiagnostics();
    }, []);

    useEffect(() => {
        setDiagnosticUserScope(userId);
    }, [userId]);

    useEffect(() => {
        const interactiveSelector = 'button, [role="button"], a, [class*="card"], [class*="Card"]';
        const blurPointerFocus = (event: PointerEvent) => {
            if (event.pointerType === "mouse" && event.button !== 0) return;
            const target = event.target instanceof Element ? event.target.closest<HTMLElement>(interactiveSelector) : null;
            if (!target || target.hasAttribute("disabled") || target.getAttribute("aria-disabled") === "true") return;
            // 浏览器可能把鼠标点击误判为 :focus-visible；下一帧只清掉这次指针点击产生的焦点。
            window.requestAnimationFrame(() => {
                if (document.activeElement === target) target.blur();
            });
        };
        document.addEventListener("pointerdown", blurPointerFocus, true);
        return () => document.removeEventListener("pointerdown", blurPointerFocus, true);
    }, []);

    useEffect(() => {
        if (handledConfigParams.current) return;
        const searchParams = new URLSearchParams(window.location.search);
        const baseUrl = searchParams.get("baseUrl") || searchParams.get("baseurl");
        const ignoredApiKey = searchParams.has("apiKey") || searchParams.has("apikey");
        if (!baseUrl && !ignoredApiKey) return;
        handledConfigParams.current = true;
        searchParams.delete("baseUrl");
        searchParams.delete("baseurl");
        searchParams.delete("apiKey");
        searchParams.delete("apikey");
        window.history.replaceState(null, "", `${window.location.pathname}${searchParams.size ? `?${searchParams}` : ""}${window.location.hash}`);
        message.warning("渠道配置链接暂不可用，请在模型设置中选择平台模型");
    }, [message]);

    return <>{children}</>;
}
