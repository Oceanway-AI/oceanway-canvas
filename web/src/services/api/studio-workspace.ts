import { appRoute } from "@/lib/app-path";
import { platformLoginNext, readPlatformLoginNext } from "@/lib/platform-login";
import { http } from "./request";

export type StudioWorkspaceContext = {
    userId: string;
    workspaceId: string;
    workspaceKind: "personal" | "team";
    workspaceName: string;
    membershipGeneration: number;
    supported: boolean;
};

export function getStudioWorkspaceContext() {
    let url = new URL(window.location.href);
    const route = appRoute(url.pathname);
    if (route === "/platform/login/complete") url = new URL(readPlatformLoginNext(), url.origin);
    else if (route === "/login") url = new URL(platformLoginNext(url.searchParams.get("next")), url.origin);
    const params = new URLSearchParams();
    // Preserve repeated intent values so the server can reject ambiguity.
    for (const key of ["workspace_id", "membership_generation"]) {
        const values = url.searchParams.getAll(key);
        for (const value of values) params.append(key, value);
    }
    const canvas = /^\/canvas\/([^/]+)\/?$/.exec(appRoute(url.pathname));
    if (canvas) params.set("canvas_id", decodeURIComponent(canvas[1]));
    return http.get<StudioWorkspaceContext>("/auth/workspace-context", { params });
}
