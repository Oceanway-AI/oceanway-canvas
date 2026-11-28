import { getFeatureAvailability, type AuthSessionPayload } from "@/services/api/auth";
import { projectImageParameters, imageParametersForSpec } from "./image-parameters";
import { getModelCatalog, getModelCatalogPage, getModelCatalogDetail, type CapabilitySpec, type ModelCatalogResponse, type OptionConstraint, type PublicChannelCatalog, type PublicLogicalModel } from "@/services/api/logical-models";
import { platformModelVersion } from "@/lib/platform-models";
import { decodeChannelModel, encodeChannelModel } from "@/stores/config-model-options";
import { localForageStorage } from "@/lib/localforage-storage";
import { appQueryClient } from "@/lib/query-client";
import { scopedLocalStorage, setActiveUserScope, withUserScopedPersistenceSuppressed } from "@/lib/user-scope";
import { CANVAS_STORE_KEY, flushCanvasStorePersistence, useCanvasStore, withCanvasStorePersistenceSuppressed } from "@/stores/canvas/use-canvas-store";
import { CANVAS_HISTORY_STORE_KEY, useCanvasHistoryStore } from "@/stores/canvas/use-canvas-history-store";
import { ASSET_STORE_KEY, flushAssetStorePersistence, useAssetStore, withAssetStorePersistenceSuppressed } from "@/stores/use-asset-store";
import { CONFIG_STORE_KEY, defaultConfig, modelOptionName, resolveModelChannel, normalizeConfigSnapshot, useConfigStore, type ModelCapability, type ModelChannel } from "@/stores/use-config-store";
import { CREATION_PREFERENCES_STORE_KEY, useCreationPreferencesStore } from "@/stores/use-creation-preferences-store";
import { defaultModelCapabilityConfig, STANDARD_IMAGE_SIZE_VALUES, type ModelCapabilityConfig } from "@/lib/model-capabilities";
import { isTextWire } from "@/lib/model-protocols";
import { imageSizeConfigWithPresets } from "@/lib/image-size-presets";
import { useUserStore } from "@/stores/use-user-store";
import { PLUGIN_STORE_KEY, usePluginStore } from "@/stores/use-plugin-store";
import { useGlobalAgentStore } from "@/stores/use-global-agent-store";
import { initializeRemoteUserDataSession, installRemoteUserDataAutoSync, resetRemoteUserDataSync, withRemoteUserDataSyncExclusive } from "@/services/user-data-sync";
import { clearResourceAccessCache } from "@/services/api/resources";
import { clearResourceBlobCache } from "@/services/resource-blob-cache";
import { withGenerationConsumersPaused } from "@/services/generation-consumer-lifecycle";
import { recordDiagnosticEvent } from "@/services/diagnostics/client-diagnostics";
import { useModelCatalogStore } from "@/stores/use-model-catalog-store";
import { getStudioWorkspaceContext } from "@/services/api/studio-workspace";
import { clearPickerPreview, readPickerPreview, writePickerPreview, type PickerPreviewRow } from "@oceanway/model-picker/preview-cache";

export async function switchUserStorageScope(userId?: string | null) {
    await withGenerationConsumersPaused(async () => {
        await withRemoteUserDataSyncExclusive(async () => {
            await Promise.all([flushCanvasStorePersistence(), flushAssetStorePersistence()]);
            resetRemoteUserDataSync();
            clearResourceAccessCache();
            clearResourceBlobCache();
            setActiveUserScope(userId);
        });
    });
}

let sessionGeneration = 0;
let hydratingCatalogGeneration: number | null = null;
let catalogRequest: { generation: number; userId: string; promise: Promise<void> } | null = null;
let catalogSnapshot: ModelCatalogResponse | null = null;
let catalogEpoch = 0;
const catalogCursors = new Set<string>();
let startupExtra: { channel: string; name: string } | undefined;
const catalogDetails = new Map<string, Promise<void>>();

export async function applyUserSession(payload: AuthSessionPayload) {
    const generation = ++sessionGeneration;
    hydratingCatalogGeneration = generation;
    catalogRequest = null;
    catalogSnapshot = null;
    catalogEpoch++;
    catalogDetails.clear();
    useModelCatalogStore.setState({ status: payload.user ? "loading" : "idle", error: null, preview: [], hasMore: false, loadingMore: false });
    if (!payload.user) clearPickerPreview("studio");
    withUserScopedPersistenceSuppressed(() => useConfigStore.getState().invalidateSystemChannels());
    const previousUserId = useUserStore.getState().user?.id || "";
    const nextUserId = payload.user?.id || "";
    useUserStore.setState({ hydrated: false, workspaceContext: null, workspaceError: null });
    // Query key 不携带用户 ID；身份变化时必须取消并清空旧账号请求，避免跨账号复用内存数据。
    if (previousUserId !== nextUserId) appQueryClient.clear();
    // Flush the old actor's drafts, then leave its namespace before checking
    // the next entry. No personal cache is read while team intent is unresolved.
    await switchUserStorageScope(null);
    // 切换存储 scope 时 user store 仍保留旧身份；此处只能先检查 generation。
    if (!isCurrentGeneration(generation)) return;
    withCanvasStorePersistenceSuppressed(() =>
        withAssetStorePersistenceSuppressed(() =>
            withUserScopedPersistenceSuppressed(() => {
                resetUserScopedMemory();
            }),
        ),
    );
    useUserStore.getState().setUser(payload.user);
    useUserStore.setState({ platformIdentity: payload.provider === "platform" });
    useUserStore.getState().setRuntimeLimits(payload.runtimeLimits);
    useUserStore.getState().setDrawingEngine(payload.drawingEngine);
    useUserStore.getState().setFeatures(payload.features);
    let firstPage: Promise<ModelCatalogResponse> | undefined;
    if (payload.provider === "platform" && payload.user) {
        try {
            const context = await getStudioWorkspaceContext();
            if (!isCurrentSession(generation, nextUserId)) return;
            if (context.userId !== nextUserId || !context.workspaceId || !["personal", "team"].includes(context.workspaceKind) || !Number.isSafeInteger(context.membershipGeneration) || context.membershipGeneration < 0)
                throw new Error("暂时无法确认当前空间，请重试。");
            if (!context.supported || context.workspaceKind !== "personal" || context.membershipGeneration !== 0) throw new Error("Studio 团队空间尚未开放。请返回 Console 选择可用空间后重新进入。");
            useUserStore.setState({ workspaceContext: context });
        } catch (error) {
            if (!isCurrentSession(generation, nextUserId)) return;
            useUserStore.setState({ workspaceError: error instanceof Error ? error.message : "暂时无法确认当前空间，请重试。" });
            throw error;
        }
    }
    await switchUserStorageScope(payload.user?.id);
    if (!isCurrentSession(generation, nextUserId)) return;
    if (payload.user && payload.provider === "platform") {
        firstPage = getModelCatalogPage();
        // The request can reject before local preferences finish hydrating.
        void firstPage.catch(() => undefined);
    }
    installRemoteUserDataAutoSync();
    try {
        // 缓存恢复和同步基线必须先于工作区开放，防止生成/编辑抢跑，或被初始化误记为已同步。
        await hydrateUserSessionData(payload, generation, firstPage);
    } catch (error) {
        if (isCurrentSession(generation, nextUserId)) console.warn("用户工作区后台初始化失败，基础页面仍可继续", error);
    } finally {
        if (isCurrentSession(generation, nextUserId)) useUserStore.getState().setHydrated(true);
    }
}

function resetUserScopedMemory() {
    useCanvasStore.setState({ projects: [], hydrated: false });
    useCanvasHistoryStore.setState({ deletedProjects: [] });
    useAssetStore.setState({ assets: [], hydrated: false });
    useConfigStore.setState({ config: normalizeConfigSnapshot({ config: defaultConfig }).config, pendingModelSelections: {} });
    usePluginStore.setState({ hydrated: false, installations: [], runtimeStatuses: {}, pluginStates: {} });
    useCreationPreferencesStore.setState({ hydrated: false, preferences: {} });
    // 全局 Agent 的开合与待投递消息属于账号会话，切换账号必须清空。
    useGlobalAgentStore.getState().reset();
}

async function hydrateUserSessionData(payload: AuthSessionPayload, generation: number, firstPage?: Promise<ModelCatalogResponse>) {
    const startedAt = performance.now();
    const userId = payload.user?.id || "";
    const [canvas, canvasHistory, assets, plugins] = await Promise.allSettled([
        localForageStorage.getItem(CANVAS_STORE_KEY),
        localForageStorage.getItem(CANVAS_HISTORY_STORE_KEY),
        localForageStorage.getItem(ASSET_STORE_KEY),
        localForageStorage.getItem(PLUGIN_STORE_KEY),
    ]);
    if (!isCurrentSession(generation, userId)) return;
    const persistedConfig = safeScopedStorageGet(CONFIG_STORE_KEY);
    const persistedCreationPreferences = safeScopedStorageGet(CREATION_PREFERENCES_STORE_KEY);
    await Promise.allSettled([
        useCanvasStore.persist.rehydrate(),
        useCanvasHistoryStore.persist.rehydrate(),
        useAssetStore.persist.rehydrate(),
        useConfigStore.persist.rehydrate(),
        usePluginStore.persist.rehydrate(),
        useCreationPreferencesStore.persist.rehydrate(),
    ]);
    if (!isCurrentSession(generation, userId)) return;
    // Zustand 在目标 scope 没有快照时会保留旧内存，必须显式恢复该 scope 的空状态。
    if (!hasPersistedValue(canvas)) useCanvasStore.setState({ projects: [] });
    if (!hasPersistedValue(canvasHistory)) useCanvasHistoryStore.setState({ deletedProjects: [] });
    if (!hasPersistedValue(assets)) useAssetStore.setState({ assets: [] });
    if (!hasPersistedValue(plugins)) usePluginStore.setState({ installations: [], runtimeStatuses: {}, pluginStates: {} });
    if (!persistedCreationPreferences) useCreationPreferencesStore.setState({ preferences: {} });

    // Saved selection IDs are preferences, not current permission.
    // Read the catalog independently of asset/project initialization.
    useConfigStore.getState().invalidateSystemChannels();
    hydratingCatalogGeneration = null;
    if (userId) void refreshCurrentSessionCatalog(generation, userId, !persistedConfig, firstPage).catch(() => undefined);
    else useModelCatalogStore.setState({ status: "idle", error: null });

    if (userId) {
        await initializeRemoteUserDataSession(userId);
    } else {
        resetRemoteUserDataSync();
    }

    if (isCurrentSession(generation, userId)) {
        recordDiagnosticEvent({
            category: "navigation",
            level: "info",
            code: "startup.workspace_background_ready",
            message: "工作区后台状态初始化完成",
            durationMs: performance.now() - startedAt,
        });
    }
}

function isCurrentSession(generation: number, userId: string) {
    return generation === sessionGeneration && (useUserStore.getState().user?.id || "") === userId;
}

function isCurrentGeneration(generation: number) {
    return generation === sessionGeneration;
}

function hasPersistedValue(result: PromiseSettledResult<unknown>) {
    return result.status === "fulfilled" && result.value !== null && result.value !== undefined;
}

function safeScopedStorageGet(key: string) {
    try {
        return scopedLocalStorage.getItem(key);
    } catch (error) {
        console.warn("读取用户本地配置失败", { key, error });
        return null;
    }
}

export function refreshSystemChannels(): Promise<void> {
    const userId = useUserStore.getState().user?.id || "";
    if (!userId || hydratingCatalogGeneration === sessionGeneration) return Promise.resolve();
    return refreshCurrentSessionCatalog(sessionGeneration, userId);
}

function refreshCurrentSessionCatalog(generation: number, userId: string, initializeDefaults = false, firstPage?: Promise<ModelCatalogResponse>): Promise<void> {
    if (!isCurrentSession(generation, userId)) return Promise.resolve();
    if (catalogRequest?.generation === generation && catalogRequest.userId === userId) return catalogRequest.promise;
    const { workspaceContext, platformIdentity } = useUserStore.getState();
    const scope =
        platformIdentity && workspaceContext?.userId === userId && workspaceContext.pickerSession
            ? { product: "studio", user: Number(userId), session: workspaceContext.pickerSession, workspace: JSON.stringify([workspaceContext.workspaceId, workspaceContext.membershipGeneration]) }
            : null;
    const request = { generation, userId, promise: Promise.resolve() };
    request.promise = Promise.resolve().then(async () => {
        try {
            if (!isCurrentSession(generation, userId)) return;
            const catalog = useUserStore.getState().platformIdentity ? await loadPlatformCatalog(generation, userId, "", firstPage) : await getModelCatalog();
            if (!isCurrentSession(generation, userId)) return;
            const savedText = useConfigStore.getState().pendingModelSelections.textModel || useConfigStore.getState().config.textModel;
            const channels = modelCatalogChannels(catalog);
            if (initializeDefaults && catalog.source !== "platform") {
                // Only a missing initial config gets recommended selections.
                // Preserve custom channels added while the request was pending.
                const current = useConfigStore.getState().config;
                useConfigStore.getState().replaceConfig(
                    normalizeConfigSnapshot({
                        config: {
                            ...current,
                            channels: [...channels, ...current.channels.filter((channel) => channel.scope !== "system")],
                            imageModels: undefined,
                            videoModels: undefined,
                            textModels: undefined,
                            audioModels: undefined,
                        },
                    }).config,
                );
            } else {
                useConfigStore.getState().mergeSystemChannels(channels, Boolean(catalog.hasMore));
            }
            if (scope) {
                const rows: PickerPreviewRow[] = [];
                for (const channel of catalog.channels || [])
                    for (const model of channel.models) {
                        const kind = model.platform?.model_type;
                        if (model.available && (kind === "text" || kind === "image" || kind === "video"))
                            rows.push({ id: Number(model.id), model_type: kind, model_name: model.modelKey, display_name: model.displayName, is_default: model.platform?.is_default });
                    }
                writePickerPreview(scope, rows);
            }
            useModelCatalogStore.setState({ status: "ready", error: null, preview: [], hasMore: Boolean(catalog.hasMore) });
            if (firstPage) void preloadStartupText(generation, userId, catalogEpoch, savedText).catch(() => undefined);
        } catch (error) {
            if (!isCurrentSession(generation, userId)) return;
            recordDiagnosticEvent({
                category: "runtime",
                level: "error",
                code: "model_catalog.refresh_failed",
                message: error instanceof Error ? error.message : "模型目录读取失败",
                stack: error instanceof Error ? error.stack : undefined,
            });
            catalogSnapshot = null;
            catalogEpoch++;
            catalogDetails.clear();
            useConfigStore.getState().invalidateSystemChannels();
            if (scope) clearPickerPreview(scope.product);
            useModelCatalogStore.setState({ status: "error", error: error instanceof Error ? error.message : "模型目录读取失败，请重试。", preview: [], hasMore: false });
            throw error;
        } finally {
            if (catalogRequest === request) catalogRequest = null;
        }
    });
    catalogRequest = request;
    catalogSnapshot = null;
    catalogEpoch++;
    catalogDetails.clear();
    useConfigStore.getState().invalidateSystemChannels();
    catalogCursors.clear();
    startupExtra = undefined;
    useModelCatalogStore.setState({ status: "loading", error: null, preview: scope ? readPickerPreview(scope) : [], hasMore: false, loadingMore: false });
    return request.promise;
}

async function loadPlatformCatalog(generation: number, userId: string, cursor = "", firstPage?: Promise<ModelCatalogResponse>): Promise<ModelCatalogResponse> {
    const epoch = catalogEpoch;
    const revision = cursor ? catalogSnapshot?.catalogRevision : undefined;
    const page = await (firstPage ?? getModelCatalogPage(cursor));
    if (!isCurrentSession(generation, userId) || epoch !== catalogEpoch) throw new Error("账号会话或目录已切换");
    if (page.source !== "platform" || !page.catalogRevision || !Array.isArray(page.channels)) throw new Error("模型目录摘要响应无效");
    if (revision && revision !== page.catalogRevision) throw Object.assign(new Error("模型目录已更新，请重新加载"), { status: 409 });
    if (page.hasMore && (!page.nextCursor || catalogCursors.has(page.nextCursor))) throw new Error("模型目录分页响应无效");
    const channels = new Map((catalogSnapshot?.channels || []).map((channel) => [channel.id, channel]));
    for (const channel of page.channels) {
        const previous = channels.get(channel.id);
        const models = new Map((previous?.models || []).map((item) => [item.modelKey, item]));
        for (const item of channel.models) {
            if (startupExtra?.channel === channel.id && startupExtra.name === item.modelKey) {
                const pinned = models.get(item.modelKey);
                if (pinned?.platform && item.platform && platformModelVersion(pinned.platform) !== platformModelVersion(item.platform)) throw Object.assign(new Error("模型目录已更新，请重新加载"), { status: 409 });
                models.set(item.modelKey, item.available && pinned?.platform?.state === "detail" ? { ...item, platform: pinned.platform } : item);
                continue;
            }
            if (models.has(item.modelKey)) throw new Error("模型目录分页包含重复型号");
            models.set(item.modelKey, item);
        }
        channels.set(channel.id, { ...channel, models: [...models.values()] });
    }
    const extra = startupExtra;
    if (extra) {
        if (page.channels.some((channel) => channel.id === extra.channel && channel.models.some((item) => item.modelKey === extra.name))) startupExtra = undefined;
        else if (!page.hasMore) {
            const channel = channels.get(extra.channel);
            if (channel) channels.set(extra.channel, { ...channel, models: channel.models.filter((item) => item.modelKey !== extra.name) });
            startupExtra = undefined;
        }
    }
    catalogSnapshot = { ...page, channels: [...channels.values()] };
    if (page.nextCursor) catalogCursors.add(page.nextCursor);
    return catalogSnapshot;
}

export async function loadNextModelCatalogPage() {
    const { loadingMore, status } = useModelCatalogStore.getState();
    if (status !== "ready" || loadingMore || !catalogSnapshot?.hasMore || !catalogSnapshot.nextCursor) return;
    const generation = sessionGeneration,
        epoch = catalogEpoch,
        userId = useUserStore.getState().user?.id || "";
    useModelCatalogStore.setState({ loadingMore: true, error: null });
    try {
        const page = await loadPlatformCatalog(generation, userId, catalogSnapshot.nextCursor);
        if (epoch !== catalogEpoch || !isCurrentSession(generation, userId)) return;
        useConfigStore.getState().mergeSystemChannels(modelCatalogChannels(page), Boolean(page.hasMore));
        useModelCatalogStore.setState({ hasMore: Boolean(page.hasMore) });
    } catch (error) {
        if (epoch !== catalogEpoch || !isCurrentSession(generation, userId)) return;
        const status = (error as { status?: number })?.status;
        if (status === 409 || status === 401 || status === 403) {
            if (status === 409) await refreshCurrentSessionCatalog(generation, userId);
            else {
                catalogEpoch++;
                catalogSnapshot = null;
                catalogDetails.clear();
                clearPickerPreview("studio");
                useConfigStore.getState().invalidateSystemChannels();
                useModelCatalogStore.setState({ status: "error", error: error instanceof Error ? error.message : "无权读取模型目录", hasMore: false, loadingMore: false, preview: [] });
            }
        } else useModelCatalogStore.setState({ error: error instanceof Error ? error.message : "下一页读取失败" });
    } finally {
        if (epoch === catalogEpoch) useModelCatalogStore.setState({ loadingMore: false });
    }
}

async function preloadStartupText(generation: number, userId: string, epoch: number, selection: string) {
    const channelId = "oceanway-platform";
    const selected = decodeChannelModel(selection || "");
    if (selected && selected.channelId !== channelId) return;
    const name = selected?.model || selection || "";
    const match = (item: PublicChannelCatalog["models"][number]) => item.available && item.platform?.state === "summary" && item.platform.model_type === "text" && (name ? item.modelKey === name : item.platform.is_default);
    let channel = catalogSnapshot?.channels?.find((row) => row.id === channelId);
    if (!channel) return;
    let item = channel.models.find(match);
    if (!item && catalogSnapshot?.hasMore) {
        const filtered = await getModelCatalogPage("", { model_type: "text", ...(name ? { query: name } : { defaults_only: true }) });
        if (!isCurrentSession(generation, userId) || epoch !== catalogEpoch || filtered.source !== "platform" || !filtered.catalogRevision) return;
        channel = catalogSnapshot?.channels?.find((row) => row.id === channelId);
        item = filtered.channels?.find((row) => row.id === channelId)?.models.find(match);
        if (!channel || !item || !catalogSnapshot?.channels) return;
        const existing = channel.models.find((row) => row.modelKey === item!.modelKey);
        if (existing && !match(existing)) return;
        if (!existing && catalogSnapshot.hasMore) {
            startupExtra = { channel: channelId, name: item.modelKey };
            catalogSnapshot = { ...catalogSnapshot, channels: catalogSnapshot.channels.map((row) => (row === channel ? { ...row, models: [...row.models, item!] } : row)) };
            useConfigStore.getState().mergeSystemChannels(modelCatalogChannels(catalogSnapshot), true);
        }
    }
    if (item) await ensurePlatformModelDetail(encodeChannelModel(channelId, item.modelKey));
}

export function ensurePlatformModelDetail(modelOption: string): Promise<void> {
    const config = useConfigStore.getState().config;
    const channel = resolveModelChannel(config, modelOption);
    const model = channel.modelDetails?.find((item) => item.model === modelOptionName(modelOption))?.platform;
    if (!model || model.state !== "summary") return Promise.resolve();
    const generation = sessionGeneration;
    const epoch = catalogEpoch;
    const userId = useUserStore.getState().user?.id || "";
    const key = JSON.stringify([generation, epoch, userId, platformModelVersion(model)]);
    const existing = catalogDetails.get(key);
    if (existing) return existing;
    const promise = getModelCatalogDetail(model)
        .then((result) => {
            if (!isCurrentSession(generation, userId) || epoch !== catalogEpoch) throw new Error("模型目录或账号已更新，请重新选择");
            const detail = result.model;
            if (detail.modelKey !== model.model_name || detail.platform?.state !== "detail" || platformModelVersion(detail.platform) !== platformModelVersion(model)) throw new Error("模型详情版本不一致，请刷新目录");
            if (!catalogSnapshot?.channels) throw new Error("模型目录尚未加载");
            catalogSnapshot = {
                ...catalogSnapshot,
                channels: catalogSnapshot.channels.map((item) => (item.id === channel.id ? { ...item, models: item.models.map((entry) => (entry.modelKey === detail.modelKey ? detail : entry)) } : item)),
            };
            useConfigStore.getState().mergeSystemChannels(modelCatalogChannels(catalogSnapshot), Boolean(catalogSnapshot.hasMore));
        })
        .finally(() => {
            if (catalogDetails.get(key) === promise) catalogDetails.delete(key);
        });
    catalogDetails.set(key, promise);
    return promise;
}

// 目录仅接受系统渠道模型，避免畸形响应被当成空目录写入配置。
function modelCatalogChannels(catalog: ModelCatalogResponse): ModelChannel[] {
    if (catalog.source === "system" || catalog.source === "platform") {
        if (!Array.isArray(catalog.channels)) throw new Error("模型目录响应缺少系统渠道列表");
        return systemChannelModelChannels(catalog.channels);
    }
    throw new Error("模型目录响应来源无效");
}

// 系统渠道模型转换为前端配置格式
export function systemChannelModelChannels(channels: PublicChannelCatalog[]): ModelChannel[] {
    return channels.map((channel) => {
        const availableModels = channel.models.filter((m) => m.available);
        return {
            id: channel.id,
            name: channel.displayName,
            sortOrder: channel.sortOrder,
            platformGroups: channel.platformGroups,
            // 系统渠道必须走带渠道 ID 的站内代理；/api 只是业务 API 根路径，
            // 不能作为模型请求的运行时 Base URL 传给 channelRequest。
            baseUrl: `/api/${channel.id}`,
            apiKey: "system",
            apiFormat: "openai",
            scope: "system" as const,
            enabled: true,
            models: availableModels.map((m) => m.modelKey),
            modelAliases: {},
            modelDetails: availableModels.map((model) => {
                return {
                    platform: model.platform,
                    model: model.modelKey,
                    displayName: model.displayName,
                    channelLabel: model.channelLabel,
                    tags: model.tags || [],
                    description: model.description || "",
                    icon: model.icon || "",
                    capability: model.capability as ModelCapability,
                    protocol: model.protocol as any,
                    textWire: isTextWire(model.textWire) ? model.textWire : undefined,

                    capabilityConfig: (model.capabilityConfig as ModelCapabilityConfig | undefined) || defaultModelCapabilityConfig(),
                    defaultOptions: model.defaultOptions,
                    channelModelId: model.id,
                    channelId: channel.id,
                    modelKey: model.modelKey,

                    availability: model.availability,
                    available: model.available,
                };
            }),
        };
    });
}

export function projectLogicalCapability(spec: CapabilitySpec, defaults: Record<string, unknown>): ModelCapabilityConfig {
    const projected = defaultModelCapabilityConfig();
    if (spec.capability === "image" && projected.image) {
        projected.image.references.maxImages = spec.inputs?.image?.max ?? 0;
        projected.image.references.maskSupported = (spec.inputs?.mask?.max ?? 0) > 0;
        projected.image.size = { parameter: "none", values: [], default: "auto", allowCustom: false };
        projected.image.quality = { supported: false, values: [], default: "auto" };
        projected.image.transparentBackground = { supported: false, default: false };
        const sizeOption = spec.options?.size || spec.options?.aspectRatio;
        const sizeValues = stringValues(sizeOption);
        const sizeAllowsCustom = sizeValues.includes("*") || Boolean(spec.imageSize?.allowCustom);
        const concreteSizeValues = sizeValues.filter((value) => value !== "*");
        const sizePresets = concreteSizeValues.length ? concreteSizeValues : sizeAllowsCustom ? [...STANDARD_IMAGE_SIZE_VALUES] : [];
        if (sizePresets.length || sizeAllowsCustom || spec.imageSize?.presets?.length) {
            const parameter = spec.imageSize?.parameter === "aspect_ratio" || spec.imageSize?.parameter === "size" ? spec.imageSize.parameter : "size";
            projected.image.size = { parameter, values: sizePresets, default: concreteDefault(defaults.size, sizePresets, "1:1"), allowCustom: sizeAllowsCustom };
            if (spec.imageSize?.presets?.length) projected.image.size = imageSizeConfigWithPresets(projected.image, spec.imageSize.presets);
        }
        applyStringOption(spec.options?.quality, defaults.quality, (values, initial) => {
            projected.image!.quality = { supported: true, values, default: initial };
        });
        projected.image.maxOutputs = maxNumericOption(spec.options?.count, 1);
        projected.image.transparentBackground = booleanOption(spec.options?.transparentBackground, defaults.transparentBackground);
        if (spec.imageParameters)
            projected.image = projectImageParameters(
                projected.image,
                imageParametersForSpec(spec).map((field) => ({ ...field, default: (defaults[field.name] as import("./image-parameters").ImageParameterValue | undefined) ?? field.default })),
            );
    }
    if (spec.capability === "video" && projected.video) {
        projected.video.references.minImages = spec.inputs?.image?.min ?? 0;
        projected.video.references.maxImages = spec.inputs?.image?.max ?? 0;
        projected.video.references.maxVideos = spec.inputs?.video?.max ?? 0;
        projected.video.references.maxAudios = spec.inputs?.audio?.max ?? 0;
        projected.video.operations = spec.operations || [];
        projected.video.defaultOperation = spec.operations?.[0] || "";
        const duration = spec.options?.videoSeconds || spec.options?.duration;
        if (duration?.values?.length) projected.video.duration = { selection: "enum", values: duration.values.map(Number).filter(Number.isFinite), default: Number(defaults.videoSeconds ?? duration.values[0]) };
        else if (duration?.min !== undefined && duration.max !== undefined) projected.video.duration = { selection: "range", min: duration.min, max: duration.max, step: duration.step || 1, default: Number(defaults.videoSeconds ?? duration.min) };
        projected.video.ratios = stringValues(spec.options?.size || spec.options?.aspectRatio);
        projected.video.defaultRatio = concreteDefault(defaults.size, projected.video.ratios, "");
        projected.video.resolutions = stringValues(spec.options?.vquality || spec.options?.resolution);
        projected.video.defaultResolution = String(defaults.vquality ?? projected.video.resolutions[0] ?? "");
        projected.video.generateAudio = booleanOption(spec.options?.videoGenerateAudio, defaults.videoGenerateAudio);
        projected.video.watermark = booleanOption(spec.options?.videoWatermark, defaults.videoWatermark);
    }
    return projected;
}

function applyStringOption(option: OptionConstraint | undefined, fallback: unknown, apply: (values: string[], initial: string) => void) {
    const values = stringValues(option);
    if (values.length) apply(values, concreteDefault(fallback, values, values[0]));
}

function stringValues(option?: OptionConstraint) {
    return (option?.values || [])
        .map(String)
        .map((value) => value.trim())
        .filter(Boolean);
}

function concreteDefault(value: unknown, values: string[], fallback: string) {
    const candidate = String(value ?? "").trim();
    return candidate && candidate !== "*" && values.includes(candidate) ? candidate : values.find((item) => item !== "*") || fallback;
}

function maxNumericOption(option: OptionConstraint | undefined, fallback: number) {
    if (option?.max !== undefined) return option.max;
    const values = (option?.values || []).map(Number).filter(Number.isFinite);
    return values.length ? Math.max(...values) : fallback;
}

function booleanOption(option: OptionConstraint | undefined, fallback: unknown) {
    const supported = (option?.values || []).some((value) => value === true || value === "true");
    return { supported, default: supported && String(fallback) === "true" };
}

export async function refreshFeatureAvailability() {
    const payload = await getFeatureAvailability();
    useUserStore.getState().setFeatures(payload.features);
    return payload.features;
}
