import { lazy, Suspense, type ReactNode } from "react";
import { createBrowserRouter, Navigate, Outlet, useLocation } from "react-router";

import { RequireAuth } from "@/components/auth/require-auth";
import { StudioRouteBoundary } from "@/components/auth/studio-route-boundary";
// 功能开关守卫包在页面 Suspense 之外，懒加载会让整个工作区退回全屏加载页；体积很小，直接静态引入。
import { RequireFeature } from "@/components/auth/require-feature";
import { FullScreenLoader, WorkspaceRouteLoader } from "@/components/ui/aceternity/full-screen-loader";
import { workspaceRoutePages } from "@/lib/workspace-route-modules";
import { CanvasRefreshShell } from "@/pages/canvas/canvas-refresh-shell";
import RouteErrorPage from "@/pages/route-error";
import { isGuestWorkspacePath } from "@/lib/guest-workspace";
import { appBasePath } from "@/lib/app-path";

const AssetsPage = workspaceRoutePages.assets;
const LoginPage = lazy(() => import("@/pages/auth/login"));
const PlatformLoginComplete = lazy(() => import("@/pages/auth/platform-login-complete"));
const CanvasPage = workspaceRoutePages.canvas;
const CanvasProjectPage = workspaceRoutePages.canvasProject;
const SharedCanvasPage = lazy(() => import("@/pages/canvas/shared"));
const ArkPortraitResultPage = lazy(() => import("@/pages/ark-portrait-result"));
const CreatePage = workspaceRoutePages.create;
const EcommercePage = workspaceRoutePages.ecommerce;
const ImageWorkbenchPage = workspaceRoutePages.imageWorkbench;
const VideoWorkbenchPage = workspaceRoutePages.videoWorkbench;
const NotFound = lazy(() => import("@/pages/not-found"));
const SkillsPage = workspaceRoutePages.skills;
const SkillWorkbenchPage = workspaceRoutePages.skillWorkbench;
const PluginsPage = workspaceRoutePages.plugins;
const EagleLibraryPage = workspaceRoutePages.eagleLibrary;
const TasksPage = workspaceRoutePages.tasks;
const ProjectsPage = workspaceRoutePages.projects;
const ProjectDetailPage = workspaceRoutePages.projectDetail;
const ProjectCreatePage = workspaceRoutePages.projectCreate;
const SettingsPage = workspaceRoutePages.settings;
const TestVoiceRecording = lazy(() => import("@/pages/test-voice-recording"));
const UserLayout = lazy(() => import("@/layouts/user-layout"));

function deferred(element: ReactNode) {
    return <Suspense fallback={<WorkspaceRouteLoader />}>{element}</Suspense>;
}

function fullScreenDeferred(element: ReactNode) {
    return <Suspense fallback={<FullScreenLoader label="正在打开创作空间" detail="准备当前页面" />}>{element}</Suspense>;
}

function AuthenticatedWorkspaceLayout() {
    const { pathname } = useLocation();
    const isCanvasProjectRoute = pathname.startsWith("/canvas/");
    const fallback = isCanvasProjectRoute ? <CanvasRefreshShell /> : <FullScreenLoader label="正在打开创作空间" detail="准备当前页面" />;
    return (
        <RequireAuth allowAnonymous={isGuestWorkspacePath(pathname)}>
            <Suspense fallback={fallback}>
                <UserLayout>
                    <StudioRouteBoundary>
                        <Outlet />
                    </StudioRouteBoundary>
                </UserLayout>
            </Suspense>
        </RequireAuth>
    );
}

/**
 * DEV 专用实验室路由。
 *
 * lazy(() => import(...)) 写在函数体内，而不是模块顶层常量：
 * 生产构建时 import.meta.env.DEV 被替换为 false，本函数随之不可达，
 * 摇树会连同其中的动态 import 一起删除，实验室代码不进入生产依赖图。
 * 若把 lazy 提到模块顶层，动态 import 会被静态分析成真实 chunk 并打进 dist。
 */
function devRoutes() {
    const FolderPreviewLab = lazy(() => import("@/pages/dev/folder-preview-lab"));
    const PrevisReproLab = lazy(() => import("@/pages/dev/previs-repro-lab"));
    return [
        { path: "/dev/folders", element: fullScreenDeferred(<FolderPreviewLab />), errorElement: <RouteErrorPage /> },
        { path: "/dev/previs-repro", element: fullScreenDeferred(<PrevisReproLab />), errorElement: <RouteErrorPage /> },
    ];
}

export const router = createBrowserRouter(
    [
        {
            errorElement: <RouteErrorPage />,
            children: [
                { path: "/login", element: fullScreenDeferred(<LoginPage />) },
                { path: "/platform/login/complete", element: fullScreenDeferred(<PlatformLoginComplete />) },
                { path: "/register", element: fullScreenDeferred(<LoginPage />) },
                { path: "/forgot-password", element: fullScreenDeferred(<LoginPage />) },
            ],
        },
        { path: "/share/canvas/:token", element: <StudioRouteBoundary>{fullScreenDeferred(<SharedCanvasPage />)}</StudioRouteBoundary>, errorElement: <RouteErrorPage /> },
        // H5 真人认证回跳页：多在未登录的手机上打开，不进入工作区登录守卫。
        { path: "/ark-portrait/result", element: <StudioRouteBoundary>{fullScreenDeferred(<ArkPortraitResultPage />)}</StudioRouteBoundary>, errorElement: <RouteErrorPage /> },
        ...(import.meta.env.DEV ? devRoutes() : []),
        {
            element: <AuthenticatedWorkspaceLayout />,
            errorElement: <RouteErrorPage />,
            children: [
                { path: "/", element: deferred(<CreatePage />) },
                { path: "/create", element: deferred(<CreatePage />) },
                { path: "/ecommerce", element: <RequireAuth>{deferred(<EcommercePage />)}</RequireAuth> },
                {
                    path: "/toolbox/image",
                    element: (
                        <RequireAuth>
                            <RequireFeature feature="imageWorkbenchEnabled">{deferred(<ImageWorkbenchPage />)}</RequireFeature>
                        </RequireAuth>
                    ),
                },
                {
                    path: "/toolbox/video",
                    element: (
                        <RequireAuth>
                            <RequireFeature feature="videoWorkbenchEnabled">{deferred(<VideoWorkbenchPage />)}</RequireFeature>
                        </RequireAuth>
                    ),
                },
                {
                    path: "/tasks",
                    element: (
                        <RequireAuth>
                            <RequireFeature feature="taskCenterEnabled">{deferred(<TasksPage />)}</RequireFeature>
                        </RequireAuth>
                    ),
                },
                { path: "/assets", element: <RequireAuth>{deferred(<AssetsPage />)}</RequireAuth> },
                { path: "/skills", element: <RequireAuth>{deferred(<SkillsPage />)}</RequireAuth> },
                { path: "/skills/:skillId/edit", element: <RequireAuth>{deferred(<SkillWorkbenchPage />)}</RequireAuth> },
                {
                    path: "/plugins",
                    element: (
                        <RequireAuth>
                            <RequireFeature feature="pluginCenterEnabled">{deferred(<PluginsPage />)}</RequireFeature>
                        </RequireAuth>
                    ),
                },
                {
                    path: "/plugins/eagle",
                    element: (
                        <RequireAuth>
                            <RequireFeature feature="pluginCenterEnabled">{deferred(<EagleLibraryPage />)}</RequireFeature>
                        </RequireAuth>
                    ),
                },
                {
                    path: "/wallet",
                    element: <RequireAuth>{null}</RequireAuth>,
                },
                { path: "/settings", element: <RequireAuth>{deferred(<SettingsPage />)}</RequireAuth> },
                { path: "/referral", element: null },
                { path: "/test-voice-recording", element: <RequireAuth>{deferred(<TestVoiceRecording />)}</RequireAuth> },
                {
                    path: "/projects",
                    element: (
                        <RequireAuth>
                            <RequireFeature feature="shortDramaEnabled">{deferred(<ProjectsPage />)}</RequireFeature>
                        </RequireAuth>
                    ),
                },
                {
                    path: "/projects/new",
                    element: (
                        <RequireAuth>
                            <RequireFeature feature="shortDramaEnabled">{deferred(<ProjectCreatePage />)}</RequireFeature>
                        </RequireAuth>
                    ),
                },
                {
                    path: "/projects/:projectId",
                    element: (
                        <RequireAuth>
                            <RequireFeature feature="shortDramaEnabled">{deferred(<ProjectDetailPage />)}</RequireFeature>
                        </RequireAuth>
                    ),
                },
                {
                    path: "/projects/:projectId/e/:episodeId",
                    element: (
                        <RequireAuth>
                            <RequireFeature feature="shortDramaEnabled">{deferred(<ProjectDetailPage />)}</RequireFeature>
                        </RequireAuth>
                    ),
                },
                { path: "/projects/:projectId/*", element: <Navigate to=".." relative="path" replace /> },
                { path: "/canvas", element: <RequireAuth>{deferred(<CanvasPage />)}</RequireAuth> },
                {
                    path: "/canvas/:id",
                    element: (
                        <RequireAuth>
                            <CanvasProjectPage />
                        </RequireAuth>
                    ),
                },
                { path: "/admin/*", element: null },
            ],
        },
        { path: "*", element: fullScreenDeferred(<NotFound />) },
    ],
    { basename: appBasePath || "/" },
);
