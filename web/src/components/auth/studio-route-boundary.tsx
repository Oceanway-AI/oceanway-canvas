import type { ReactNode } from "react";
import { Link, useLocation } from "react-router";
import { WorkspacePage } from "@/components/layout/workspace-page";
import { WorkspaceState } from "@/components/layout/workspace-state";
import { studioSettingsSectionAvailable, studioUnavailableRoute } from "@/lib/studio-release-surface";

export function StudioRouteBoundary({ children }: { children: ReactNode }) {
    const { pathname, search } = useLocation();
    const section = new URLSearchParams(search).get("section");
    const unavailable = studioUnavailableRoute(pathname) || (pathname === "/settings" && section && !studioSettingsSectionAvailable(section) ? "此设置" : "");
    if (!unavailable) return children;
    return (
        <WorkspacePage>
            <WorkspaceState kind="forbidden" icon="settings" title={`${unavailable}暂不可用`} description="此功能尚未开放。你可以继续使用画布、素材和 Agent 创作。" action={<Link to="/canvas">返回画布</Link>} />
        </WorkspacePage>
    );
}
