import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { AudioLines, FileText, Folder, Image, Plus, Search, Video } from "lucide-react";
import { useNavigate } from "react-router";
import { listProjects } from "@/services/api/projects";
import { listRemoteAssetsPage } from "@/services/api/user-data";
import { loadPlatformLibraryPage } from "@/services/platform-library-assets";
import { useUserStore } from "@/stores/use-user-store";
import { RelativeTime } from "@/components/ui/product/relative-time";

const tabs = [
    { id: "projects", label: "项目", icon: Folder },
    { id: "image", label: "图片", icon: Image },
    { id: "video", label: "视频", icon: Video },
    { id: "audio", label: "音频", icon: AudioLines },
    { id: "text", label: "文档", icon: FileText },
] as const;
type ContextTab = (typeof tabs)[number]["id"];

export function OceanwayContextPanel({ onRequireLogin }: { onRequireLogin: () => void }) {
    const [tab, setTab] = useState<ContextTab>("projects");
    const userId = useUserStore((state) => state.user?.id);
    const platform = useUserStore((state) => state.platformIdentity);
    const projectsEnabled = useUserStore((state) => !state.platformIdentity && state.features.shortDramaEnabled);
    const activeTab = tab === "projects" && !projectsEnabled ? "image" : tab;
    const visibleTabs = tabs.filter((item) => projectsEnabled || item.id !== "projects");
    const ready = useUserStore((state) => state.hydrated);
    const navigate = useNavigate();
    const projects = useQuery({
        queryKey: ["oceanway-home-projects", userId],
        queryFn: () => listProjects({ page: 1, pageSize: 3 }),
        enabled: Boolean(userId) && ready && activeTab === "projects",
        staleTime: 30_000,
    });
    const assets = useQuery({
        queryKey: ["asset-library", "oceanway-home", userId, platform, activeTab],
        queryFn: async ({ signal }) => {
            const options = { page: 1, pageSize: 3, kind: platform && activeTab === "text" ? "document" : activeTab, signal };
            const page = platform ? await loadPlatformLibraryPage(options) : await listRemoteAssetsPage(options);
            return page.assets.map(({ id, title, updatedAt, coverUrl }) => ({ id, title, updatedAt, coverUrl }));
        },
        enabled: Boolean(userId) && ready && activeTab !== "projects",
        staleTime: 30_000,
    });
    const open = (path: string) => (userId ? navigate(path) : onRequireLogin());
    const destination = activeTab === "projects" ? "/projects" : "/assets";
    const query = activeTab === "projects" ? projects : assets;
    const Icon = tabs.find((item) => item.id === activeTab)!.icon;
    const items =
        activeTab === "projects"
            ? (projects.data?.projects || []).map(({ project, canvasCount, assetCount }) => ({
                  id: project.id,
                  title: project.name,
                  coverUrl: "",
                  updatedAt: project.updatedAt,
                  detail: `${canvasCount} 个画布 · ${assetCount} 份素材`,
                  path: `/projects/${encodeURIComponent(project.id)}`,
              }))
            : (assets.data || []).map((asset) => ({ ...asset, detail: "", path: "/assets" }));
    return (
        <section className="oceanway-context-panel" aria-labelledby="oceanway-context-title">
            <header className="oceanway-context-header">
                <h2 id="oceanway-context-title">{projectsEnabled ? "项目与素材" : "创作素材"}</h2>
                <button type="button" aria-label="打开项目与素材" onClick={() => open(destination)}>
                    <Search />
                </button>
            </header>
            <div className="oceanway-context-tabs" role="group" aria-label="项目与素材分类">
                {visibleTabs.map((item) => (
                    <button type="button" key={item.id} aria-pressed={activeTab === item.id} onClick={() => setTab(item.id)}>
                        {item.label}
                    </button>
                ))}
            </div>
            {!userId ? (
                <div className="oceanway-context-items">
                    <button type="button" className="oceanway-context-item" onClick={onRequireLogin}>
                        <Folder />
                        <span>
                            <strong>你的创作项目</strong>
                            <small>登录后，继续上一次的灵感</small>
                        </span>
                    </button>
                    <button type="button" className="oceanway-context-item" onClick={onRequireLogin}>
                        <Image />
                        <span>
                            <strong>统一素材空间</strong>
                            <small>让图片、视频与音频随手可用</small>
                        </span>
                    </button>
                    <button type="button" className="oceanway-context-item" onClick={onRequireLogin}>
                        <Plus />
                        <span>
                            <strong>开启新的创作</strong>
                            <small>从一个想法，走向完整作品</small>
                        </span>
                    </button>
                </div>
            ) : query.isPending ? (
                <div className="oceanway-context-state" role="status">
                    正在加载{activeTab === "projects" ? "项目" : "素材"}…
                </div>
            ) : query.isError ? (
                <div className="oceanway-context-state" role="alert">
                    <span>暂时无法加载，请重试</span>
                    <button type="button" onClick={() => void query.refetch()}>
                        重新加载
                    </button>
                </div>
            ) : items.length ? (
                <div className="oceanway-context-items">
                    {items.map((item) => (
                        <button type="button" key={item.id} className="oceanway-context-item" onClick={() => navigate(item.path)}>
                            {activeTab === "image" || activeTab === "video" ? (
                                <span className="oceanway-context-cover">
                                    <Icon aria-hidden />
                                    {item.coverUrl && (
                                        <img
                                            key={item.coverUrl}
                                            src={item.coverUrl}
                                            alt=""
                                            loading="lazy"
                                            decoding="async"
                                            onError={(event) => {
                                                event.currentTarget.hidden = true;
                                            }}
                                        />
                                    )}
                                </span>
                            ) : (
                                <Icon />
                            )}
                            <span>
                                <strong>{item.title}</strong>
                                <small>
                                    {item.detail ? `${item.detail} · ` : ""}
                                    <RelativeTime value={item.updatedAt} />
                                </small>
                            </span>
                        </button>
                    ))}
                </div>
            ) : (
                <div className="oceanway-context-state">
                    <Icon size={24} />
                    <span>{activeTab === "projects" ? "从你的第一个项目开始" : "创作素材将在这里汇聚"}</span>
                    <button type="button" onClick={() => navigate(activeTab === "projects" ? "/projects/new" : "/assets")}>
                        {activeTab === "projects" ? "新建项目" : "打开素材库"}
                    </button>
                </div>
            )}
        </section>
    );
}
