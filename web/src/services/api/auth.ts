import type { CanvasDrawingEngineSetting } from "@/lib/canvas/canvas-drawing-engine";
import type { LocalUser, RuntimeLimits, FeatureAvailability } from "@/stores/use-user-store";
import { http } from "@/services/api/request";

export type { LocalUser } from "@/stores/use-user-store";

export type AuthSessionPayload = {
    user: LocalUser | null;
    provider?: "platform";
    runtimeLimits?: RuntimeLimits;
    drawingEngine?: CanvasDrawingEngineSetting;
    features?: FeatureAvailability;
};

let authSessionRequest: Promise<AuthSessionPayload> | null = null;
let authSessionCache: { payload: AuthSessionPayload; expiresAt: number } | null = null;
let authSessionGeneration = 0;

export function invalidateAuthSessionCache() {
    authSessionGeneration++;
    authSessionCache = null;
    authSessionRequest = null;
}

export type PromptTemplate = {
    id: string;
    operation: string;
    name: string;
    version: number;
    content: string;
    outputType: "json" | "text";
    enabled: boolean;
    createdBy?: string;
    createdAt: string;
    updatedAt: string;
};

export type PromptTemplateVariable = {
    label: string;
    placeholder: string;
};

export type PromptOperationDefinition = {
    operation: string;
    label: string;
    category: string;
    description: string;
    outputType: "json" | "text";
    schemaKey?: string;
    variables: PromptTemplateVariable[];
    outputContract: string;
};

export type UserPromptCustomization = {
    id: string;
    operation: string;
    mode: "inherit" | "append" | "rewrite";
    content: string;
    baseTemplateId: string;
    updatedAt: string;
};

export type UserPromptPreference = {
    definition: PromptOperationDefinition;
    template: PromptTemplate | null;
    customization?: UserPromptCustomization;
    outdated: boolean;
};

export type AdminArkPrivateAssetSetting = {
    /** Seedance 可信参考素材自动同步。 */
    enabled: boolean;
    /** 开放用户自建真人 / 虚拟素材组，与可信素材自动同步相互独立。 */
    realEnabled: boolean;
    virtualEnabled: boolean;
    realGroupLimit: number;
    virtualGroupLimit: number;
    realReviewRequired: boolean;
    virtualReviewRequired: boolean;
    realAuthorizationDays: number;
    /** 方舟站点：火山引擎或 BytePlus 海外站，决定控制面域名。 */
    site: "volcengine" | "byteplus";
    /** 真人认证回调使用的站点公网根地址；为空时后端使用 CANVAS_PUBLIC_BASE_URL。 */
    callbackBaseUrl: string;
    region: string;
    projectName: string;
    accessKeyId: string;
    accessKeySecret?: string;
    hasAccessKeySecret: boolean;
    updatedBy?: string;
    createdAt?: string;
    updatedAt?: string;
};

export function getPlatformLoginURL(signal?: AbortSignal) {
    return http.get<{ url: string }>("/auth/login-url", { signal });
}

export function getAuthSession() {
    const now = Date.now();
    if (authSessionCache && authSessionCache.expiresAt > now) return Promise.resolve(authSessionCache.payload);
    if (authSessionRequest) return authSessionRequest;
    const generation = authSessionGeneration;
    const request = http
        .post<AuthSessionPayload>("/auth/session", {})
        .then((payload) => {
            if (generation !== authSessionGeneration) throw new Error("登录状态已变化，请重新读取");
            authSessionCache = { payload, expiresAt: Date.now() + 5_000 };
            return payload;
        })
        .finally(() => {
            if (authSessionRequest === request) authSessionRequest = null;
        });
    authSessionRequest = request;
    return authSessionRequest;
}

export function getFeatureAvailability() {
    return http.get<{ features: FeatureAvailability }>("/features");
}

export function getAdminFeatureAvailability() {
    return http.get<{ features: FeatureAvailability }>("/admin/settings/features");
}

export async function logout() {
    const result = await http.post<{ ok: boolean }>("/auth/logout");
    invalidateAuthSessionCache();
    return result;
}

export function listAdminPromptTemplates() {
    return http.get<{ templates: PromptTemplate[]; definitions: PromptOperationDefinition[] }>("/admin/prompt-templates");
}

export function createAdminPromptTemplate(input: Pick<PromptTemplate, "operation" | "name" | "content"> & { enabled?: boolean }) {
    return http.post<{ template: PromptTemplate }>("/admin/prompt-templates", input);
}

export function updateAdminPromptTemplate(id: string, input: Pick<PromptTemplate, "operation" | "name" | "content"> & { enabled?: boolean }) {
    return http.patch<{ template: PromptTemplate }>(`/admin/prompt-templates/${encodeURIComponent(id)}`, input);
}

export function deleteAdminPromptTemplate(id: string) {
    return http.delete<{ ok: boolean }>(`/admin/prompt-templates/${encodeURIComponent(id)}`);
}

export function listUserPromptPreferences() {
    return http.get<{ preferences: UserPromptPreference[] }>("/settings/prompt-templates");
}

export function updateUserPromptCustomization(operation: string, input: Pick<UserPromptCustomization, "mode" | "content">) {
    return http.patch<{ customization: UserPromptCustomization }>(`/settings/prompt-templates/${encodeURIComponent(operation)}`, input);
}

export function resetUserPromptCustomization(operation: string) {
    return http.delete<{ ok: boolean }>(`/settings/prompt-templates/${encodeURIComponent(operation)}`);
}

export function getAdminArkPrivateAssetSetting() {
    return http.get<{ setting: AdminArkPrivateAssetSetting }>("/admin/settings/ark-private-assets");
}

export function updateAdminArkPrivateAssetSetting(input: Partial<AdminArkPrivateAssetSetting>) {
    return http.patch<{ setting: AdminArkPrivateAssetSetting }>("/admin/settings/ark-private-assets", input);
}

export function getAdminDrawingEngineSetting() {
    return http.get<{ setting: CanvasDrawingEngineSetting }>("/admin/settings/drawing-engine");
}

export function updateAdminDrawingEngineSetting(input: Pick<CanvasDrawingEngineSetting, "defaultEngine" | "tldrawLicenseKey">) {
    return http.patch<{ setting: CanvasDrawingEngineSetting }>("/admin/settings/drawing-engine", input);
}
