import { expect, test } from "bun:test";
import localforage from "localforage";
import { apiClient, ApiError } from "@/services/api/request";
import { initializeRemoteUserDataSession, resetRemoteUserDataSync, saveRemoteUserDataNow, loadAssetsForUse } from "@/services/user-data-sync";
import { useUserStore } from "@/stores/use-user-store";
import { useAssetStore, flushAssetStorePersistence, type Asset } from "@/stores/use-asset-store";
import { useCanvasStore } from "@/stores/canvas/use-canvas-store";
import { setActiveUserScope, getActiveUserScope } from "@/lib/user-scope";

test("Platform identity still saves and reloads explicit native creative assets", async () => {
    const prior = { adapter: apiClient.defaults.adapter, get: localforage.getItem, set: localforage.setItem, remove: localforage.removeItem, scope: getActiveUserScope(), platform: useUserStore.getState().platformIdentity };
    const disk = new Map<string, unknown>();
    localforage.getItem = (async (key: string) => disk.get(key) ?? null) as typeof localforage.getItem;
    localforage.setItem = (async (key: string, value: unknown) => {
        disk.set(key, value);
        return value;
    }) as typeof localforage.setItem;
    localforage.removeItem = async (key: string) => {
        disk.delete(key);
    };
    let saved: Asset | undefined;
    try {
        setActiveUserScope("a1-native-sync");
        useUserStore.setState({ platformIdentity: true });
        useAssetStore.setState({ assets: [] });
        useCanvasStore.setState({ projects: [] });
        apiClient.defaults.adapter = async (config) => {
            let data: unknown;
            if (config.method === "put" && config.url === "/assets/native-text") {
                saved = JSON.parse(config.data).asset;
                data = { asset: saved };
            } else if (config.url === "/assets/native-text") {
                if (!saved) throw new ApiError("missing", { status: 404 });
                data = { asset: saved };
            } else if (config.url === "/assets/batch") data = { assets: saved ? [saved] : [] };
            else throw new Error(`unexpected request ${config.url}`);
            return { config, status: 200, statusText: "OK", headers: {}, data: { code: 0, data } };
        };
        await initializeRemoteUserDataSession("a1-native-sync");
        const asset: Asset = { id: "native-text", kind: "text", title: "创作说明", coverUrl: "", tags: [], folderId: "folder-1", createdAt: "2026-11-09T00:00:00Z", updatedAt: "2026-11-09T00:00:00Z", data: { content: "保存到原资产链" } };
        useAssetStore.setState({ assets: [asset] });
        await saveRemoteUserDataNow([]);
        expect(saved).toEqual(asset);
        useAssetStore.setState({ assets: [] });
        await initializeRemoteUserDataSession("a1-native-sync");
        await loadAssetsForUse([asset.id], { refresh: true });
        expect(useAssetStore.getState().assets).toEqual([asset]);
    } finally {
        await flushAssetStorePersistence();
        resetRemoteUserDataSync();
        useAssetStore.setState({ assets: [] });
        useUserStore.setState({ platformIdentity: prior.platform });
        setActiveUserScope(prior.scope);
        apiClient.defaults.adapter = prior.adapter;
        localforage.getItem = prior.get;
        localforage.setItem = prior.set;
        localforage.removeItem = prior.remove;
    }
});
