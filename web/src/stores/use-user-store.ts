import { create } from "zustand";

import { DEFAULT_DRAWING_ENGINE, type CanvasDrawingEngineSetting } from "@/lib/canvas/canvas-drawing-engine";
import type { StudioWorkspaceContext } from "@/services/api/studio-workspace";

export type LocalUser = {
    id: string;
    username: string;
    email?: string;
    displayName: string;
    avatarUrl?: string;
    studioPermissions?: ("content:manage" | "skills:builtin:delete" | "skills:categories:write" | "skills:curation:write")[];
    status: "active" | "disabled";
};

export type RuntimeLimits = {
    activeTaskLimit: number;
    resourceUploadMB: number;
    recycleBinRetentionDays?: number;
};

export type FeatureAvailability = {
    welcomeEnabled: boolean;
    shortDramaEnabled: boolean;
    taskCenterEnabled: boolean;
    creditsEnabled: boolean;
    customChannelsEnabled: boolean;
    frontendModelsEnabled: boolean;
    pluginCenterEnabled: boolean;
    systemPluginsVisibleToUsers: boolean;
    inspirationSourcesVisible: boolean;
    imageWorkbenchEnabled: boolean;
    videoWorkbenchEnabled: boolean;
    ecommerceEnabled: boolean;
    ecommerceVideoEnabled: boolean;
    configured?: boolean;
    updatedBy?: string;
};

export const defaultFeatureAvailability: FeatureAvailability = {
    welcomeEnabled: false,
    shortDramaEnabled: false,
    taskCenterEnabled: true,
    creditsEnabled: false,
    customChannelsEnabled: false,
    frontendModelsEnabled: false,
    pluginCenterEnabled: false,
    systemPluginsVisibleToUsers: false,
    inspirationSourcesVisible: false,
    imageWorkbenchEnabled: false,
    videoWorkbenchEnabled: false,
    ecommerceEnabled: false,
    ecommerceVideoEnabled: false,
};

type UserStore = {
    hydrated: boolean;
    workspaceContext: StudioWorkspaceContext | null;
    workspaceError: string | null;
    platformIdentity: boolean;
    user: LocalUser | null;
    runtimeLimits: RuntimeLimits;
    drawingEngine: CanvasDrawingEngineSetting;
    features: FeatureAvailability;
    setUser: (user: LocalUser | null) => void;
    setRuntimeLimits: (limits?: RuntimeLimits) => void;
    setDrawingEngine: (setting?: CanvasDrawingEngineSetting) => void;
    setFeatures: (features?: FeatureAvailability) => void;
    setHydrated: (hydrated: boolean) => void;
    clearSession: () => void;
};

export const useUserStore = create<UserStore>()((set) => ({
    hydrated: false,
    workspaceContext: null,
    workspaceError: null,
    platformIdentity: false,
    user: null,
    runtimeLimits: { activeTaskLimit: 5, resourceUploadMB: 50, recycleBinRetentionDays: 30 },
    drawingEngine: { defaultEngine: DEFAULT_DRAWING_ENGINE },
    features: defaultFeatureAvailability,
    setUser: (user) => set({ user }),
    setRuntimeLimits: (runtimeLimits) => set({ runtimeLimits: runtimeLimits || { activeTaskLimit: 5, resourceUploadMB: 50, recycleBinRetentionDays: 30 } }),
    setDrawingEngine: (drawingEngine) => set({ drawingEngine: drawingEngine || { defaultEngine: DEFAULT_DRAWING_ENGINE } }),
    setFeatures: (features) => set({ features: features ? { ...defaultFeatureAvailability, ...features } : defaultFeatureAvailability }),
    setHydrated: (hydrated) => set({ hydrated }),
    clearSession: () =>
        set({
            user: null,
            workspaceContext: null,
            workspaceError: null,
            runtimeLimits: { activeTaskLimit: 5, resourceUploadMB: 50, recycleBinRetentionDays: 30 },
            drawingEngine: { defaultEngine: DEFAULT_DRAWING_ENGINE },
            features: defaultFeatureAvailability,
        }),
}));
