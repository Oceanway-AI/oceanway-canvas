import { expect, test } from "bun:test";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { MemoryRouter } from "react-router";
import { StudioRouteBoundary } from "@/components/auth/studio-route-boundary";
import { studioSettingsSectionAvailable } from "@/lib/studio-release-surface";
import { useUserStore } from "@/stores/use-user-store";
import { isPluginEffectivelyEnabled, usePluginStore } from "@/stores/use-plugin-store";

test("首发未接通入口展示状态，直接访问不会挂载旧业务组件", () => {
    for (const route of [
        "/admin",
        "/admin/channels",
        "/ecommerce",
        "/projects/new",
        "/projects/p/e/e",
        "/skills/s/edit",
        "/plugins/eagle",
        "/toolbox/image",
        "/toolbox/video",
        "/referral",
        "/wallet",
        "/test-voice-recording",
        "/share/canvas/token",
        "/ark-portrait/result",
        "/settings?section=channels",
        "/settings?section=ark-assets",
    ]) {
        let mounted = false;
        const Child = () => {
            mounted = true;
            return createElement("p", null, "旧业务");
        };
        const html = renderToStaticMarkup(createElement(MemoryRouter, { initialEntries: [route] }, createElement(StudioRouteBoundary, null, createElement(Child))));
        expect(mounted).toBe(false);
        expect(html).toContain("暂不可用");
        expect(html).toContain("返回画布");
    }
});

test("平台会话不采纳历史插件启用状态或 fallback", () => {
    const identity = useUserStore.getState().platformIdentity;
    const statuses = usePluginStore.getState().runtimeStatuses;
    try {
        useUserStore.setState({ platformIdentity: true });
        usePluginStore.setState({ runtimeStatuses: { "external-provider": "enabled" } });
        expect(isPluginEffectivelyEnabled("external-provider", true)).toBe(false);
        expect(isPluginEffectivelyEnabled("unregistered-provider", true)).toBe(false);
    } finally {
        useUserStore.setState({ platformIdentity: identity });
        usePluginStore.setState({ runtimeStatuses: statuses });
    }
});

test("首发已接通的创作/画布/素材/任务/设置继续挂载", () => {
    for (const route of ["/", "/create", "/canvas", "/canvas/canvas-1", "/assets", "/tasks", "/settings"]) {
        const html = renderToStaticMarkup(createElement(MemoryRouter, { initialEntries: [route] }, createElement(StudioRouteBoundary, null, createElement("p", null, "已接通页面"))));
        expect(html).toContain("已接通页面");
        expect(html).not.toContain("暂不可用");
    }
    for (const section of ["profile", "preferences", "models", "storage"]) expect(studioSettingsSectionAvailable(section)).toBe(true);
    for (const section of ["channels", "runninghub", "ark-assets", "prompts", "agent-memory", "feedbacks", "diagnostics"]) expect(studioSettingsSectionAvailable(section)).toBe(false);
});
