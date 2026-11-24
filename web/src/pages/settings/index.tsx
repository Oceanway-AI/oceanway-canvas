import { App, Button } from "antd";
import { ArrowLeft, Boxes, Cloud, SlidersHorizontal, UserRound } from "lucide-react";
import { useEffect, useState, type ReactNode } from "react";
import { useNavigate, useSearchParams } from "react-router";
import { useTranslation } from "react-i18next";

import { PlatformStoragePane } from "./platform-storage-pane";
import { useStudioAccount } from "@/hooks/use-studio-account";
import { studioConsoleNavigationURL } from "@/services/api/studio-account";
import { refreshSystemChannels } from "@/lib/user-session";
import { useConfigStore, useEffectiveConfig } from "@/stores/use-config-store";
import { useUserStore } from "@/stores/use-user-store";
import { ModelDefaultGrid } from "./model-default-grid";
import { ProfilePane } from "./profile-pane";
import { PreferencesPane } from "./preferences-pane";
import "./settings-page.css";

type ConfigSectionKey = "profile" | "models" | "preferences" | "storage";
type ConfigSectionGroup = "general" | "creation";
const configSectionGroups: ConfigSectionGroup[] = ["general", "creation"];
const configSections: Array<{ key: ConfigSectionKey; group: ConfigSectionGroup; icon: ReactNode }> = [
    { key: "profile", group: "general", icon: <UserRound className="size-4" /> },
    { key: "preferences", group: "general", icon: <SlidersHorizontal className="size-4" /> },
    { key: "models", group: "creation", icon: <Boxes className="size-4" /> },
    { key: "storage", group: "creation", icon: <Cloud className="size-4" /> },
];

export function isConfigSection(value: string | null): value is ConfigSectionKey {
    return configSections.some((section) => section.key === value);
}

export default function SettingsPage() {
    const { message } = App.useApp();
    const { t } = useTranslation();
    const navigate = useNavigate();
    const [searchParams, setSearchParams] = useSearchParams();
    const requestedSection = searchParams.get("section");
    const [activeTab, setActiveTab] = useState<ConfigSectionKey>(isConfigSection(requestedSection) ? requestedSection : "profile");
    const effectiveConfig = useEffectiveConfig();
    const updateConfig = useConfigStore((state) => state.updateConfig);
    const shouldPromptContinue = searchParams.get("continue") === "1";
    const userId = useUserStore((state) => state.user?.id);
    const platformIdentity = useUserStore((state) => state.platformIdentity);
    const { account } = useStudioAccount();
    const modelsURL = studioConsoleNavigationURL(account, "models", window.location.pathname);

    useEffect(() => {
        setActiveTab(isConfigSection(requestedSection) ? requestedSection : "profile");
    }, [requestedSection]);

    useEffect(() => {
        if (!userId) return;
        let cancelled = false;
        void refreshSystemChannels().catch((error) => {
            if (!cancelled) message.warning(error instanceof Error ? t("account.settings.systemRefreshFailedWith", { error: error.message }) : t("account.settings.systemRefreshFailed"));
        });
        return () => {
            cancelled = true;
        };
    }, [message, t, userId]);

    const selectSection = (section: ConfigSectionKey) => {
        setActiveTab(section);
        const next = new URLSearchParams(searchParams);
        next.set("section", section);
        setSearchParams(next, { replace: true });
    };

    const finishConfig = () => {
        if (effectiveConfig.models.length === 0) {
            selectSection("models");
            message.error(t("account.settings.noSystemModel"));
            return;
        }
        message.success(t("account.settings.saved"));
        navigate(-1);
    };

    const panes: Record<ConfigSectionKey, ReactNode> = {
        profile: (
            <SettingsPane>
                <ProfilePane key={userId} />
            </SettingsPane>
        ),
        preferences: (
            <SettingsPane>
                <PreferencesPane />
            </SettingsPane>
        ),
        models: (
            <SettingsPane>
                <div className="settings-pane-header">
                    <div className="min-w-0">
                        <h2>{t("settings.sections.models.label")}</h2>
                        {!platformIdentity && <p>{t("account.settings.modelsDescription")}</p>}
                    </div>
                </div>
                <div className="settings-section">
                    {platformIdentity && (
                        <Button href={modelsURL} disabled={!modelsURL}>
                            {t("settings.sections.models.label")} · OceanWay
                        </Button>
                    )}
                    <ModelDefaultGrid config={effectiveConfig} audioOnly={platformIdentity} onChange={(key, model) => updateConfig(key, model)} />
                </div>
            </SettingsPane>
        ),
        storage: (
            <SettingsPane>
                <div className="settings-section">
                    <PlatformStoragePane />
                </div>
            </SettingsPane>
        ),
    };

    return (
        <main className="settings-page app-workspace-page app-user-workspace flex h-full min-h-0 flex-col text-foreground">
            {shouldPromptContinue ? (
                <div className="settings-topbar shrink-0">
                    <div className="ml-auto flex flex-wrap items-center justify-end gap-2">
                        <Button icon={<ArrowLeft className="size-4" />} onClick={() => navigate(-1)}>
                            {t("settings.back")}
                        </Button>
                        <Button type="primary" onClick={finishConfig}>
                            {t("settings.saveAndBack")}
                        </Button>
                    </div>
                </div>
            ) : null}
            <div className="settings-library-frame flex min-h-0 flex-1 flex-col md:flex-row">
                <aside className="settings-nav-panel w-full shrink-0 md:w-[200px]">
                    <nav className="thin-scrollbar flex gap-1 overflow-x-auto p-2 md:block md:p-2" aria-label={t("settings.nav")}>
                        {configSectionGroups.map((group) => {
                            const items = configSections.filter((section) => section.group === group);
                            if (!items.length) return null;
                            return (
                                <div key={group} className="settings-nav-group contents md:block md:space-y-1" role="group" aria-labelledby={`settings-nav-group-${group}`}>
                                    <p id={`settings-nav-group-${group}`} className="settings-nav-group-label hidden md:block">
                                        {t(`settings.groups.${group}`)}
                                    </p>
                                    {items.map((item) => {
                                        const selected = item.key === activeTab;
                                        return (
                                            <button
                                                key={item.key}
                                                type="button"
                                                className={`settings-nav-item flex h-9 shrink-0 items-center gap-2 rounded-md px-3 text-left text-sm transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring md:h-auto md:w-full md:items-start md:gap-3 md:py-2 ${selected ? "is-active" : "text-foreground/58 hover:bg-muted/55 hover:text-foreground"}`}
                                                onClick={() => selectSection(item.key)}
                                                aria-current={selected ? "page" : undefined}
                                            >
                                                <span className={`shrink-0 md:mt-0.5 ${selected ? "text-[var(--workspace-accent)]" : ""}`}>{item.icon}</span>
                                                <span className="min-w-0">
                                                    <span className="block whitespace-nowrap text-sm font-medium">{t(`settings.sections.${item.key}.label`)}</span>
                                                    <span className="mt-1 hidden text-[var(--fs-label)] leading-4 text-current opacity-65 md:block">{t(`settings.sections.${item.key}.description`)}</span>
                                                </span>
                                            </button>
                                        );
                                    })}
                                </div>
                            );
                        })}
                    </nav>
                </aside>
                <section className="settings-content flex min-h-0 min-w-0 flex-1 flex-col">
                    <div className="app-workspace-scroll min-h-0 flex-1 overflow-y-auto overscroll-contain px-4 py-4 md:px-6 md:py-5">
                        <div className="settings-pane-root mx-auto w-full max-w-none">{panes[activeTab]}</div>
                    </div>
                </section>
            </div>
        </main>
    );
}

function SettingsPane({ children, fill = false }: { children: ReactNode; fill?: boolean }) {
    return <div className={fill ? "settings-pane h-full" : "settings-pane"}>{children}</div>;
}
