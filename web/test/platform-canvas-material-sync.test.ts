import { afterEach, expect, test } from "bun:test";
import { ensureCanvasNodeAsset } from "@/services/project-asset-sync";
import { apiClient } from "@/services/api/request";
import { useUserStore } from "@/stores/use-user-store";
import { useAssetStore } from "@/stores/use-asset-store";
import { CanvasNodeType, type CanvasNodeData } from "@/types/canvas";

const originalAdapter = apiClient.defaults.adapter;
afterEach(() => {
    apiClient.defaults.adapter = originalAdapter;
    useUserStore.setState({ platformIdentity: false });
});

test("平台画布只校验固定资源，不写旧资产接口；文本结果直接保存在画布", async () => {
    useUserStore.setState({ platformIdentity: true });
    const assets = useAssetStore.getState().assets;
    const requests: string[] = [];
    apiClient.defaults.adapter = async (config) => {
        requests.push(config.url || "");
        if (config.url !== "/resources/fixed-video") throw new Error("不应写旧资产接口");
        return { config, status: 200, statusText: "OK", headers: {}, data: { code: 0, data: { resource: { id: "fixed-video", provider: "platform", status: "ready" } } } };
    };
    const node: CanvasNodeData = { id: "video", type: CanvasNodeType.Video, title: "镜头", position: { x: 0, y: 0 }, width: 320, height: 180, metadata: { storageKey: "resource:fixed-video" } };
    expect(await ensureCanvasNodeAsset({ canvasId: "platform-canvas", node, source: "canvas-upload" })).toMatchObject({ created: false, syncPending: false });
    expect(await ensureCanvasNodeAsset({ canvasId: "platform-canvas", node: { ...node, id: "text", type: CanvasNodeType.Text, metadata: { content: "生成文本" } }, source: "canvas-generation" })).toMatchObject({ created: false, syncPending: false });
    expect(requests).toEqual(["/resources/fixed-video"]);
    expect(useAssetStore.getState().assets).toBe(assets);
});
