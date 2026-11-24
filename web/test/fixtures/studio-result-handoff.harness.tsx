import { afterAll, expect, test } from "bun:test";
import { Window } from "happy-dom";

const dom = new Window({ url: "https://studio.example.test" });
for (const [key, value] of Object.entries({
    window: dom,
    document: dom.document,
    navigator: dom.navigator,
    HTMLElement: dom.HTMLElement,
    SVGElement: dom.SVGElement,
    Element: dom.Element,
    Node: dom.Node,
    Event: dom.Event,
    CustomEvent: dom.CustomEvent,
    ShadowRoot: dom.ShadowRoot,
    MutationObserver: dom.MutationObserver,
    ResizeObserver: dom.ResizeObserver,
    getComputedStyle: dom.getComputedStyle.bind(dom),
    IS_REACT_ACT_ENVIRONMENT: true,
}))
    Object.defineProperty(globalThis, key, { configurable: true, writable: true, value });
const { act } = await import("react");
const { createRoot } = await import("react-dom/client");
const { App } = await import("antd");
const { MediaResult } = await import("@/pages/create/creation-workspace-messages");
const { TaskResultPreview } = await import("@/pages/tasks/task-detail-modal");
const { useUserStore } = await import("@/stores/use-user-store");
const { apiClient } = await import("@/services/api/request");
const { clearResourceAccessCache } = await import("@/services/api/resources");
afterAll(() => dom.happyDOM.abort());

test("视频播放版准备中只轮询原资源，就绪后开放预览，真实加载错误仍可重试", async () => {
    useUserStore.setState({ platformIdentity: true });
    const requests: string[] = [];
    apiClient.defaults.adapter = async (config) => {
        requests.push(config.url || "");
        const [{ resourceId }] = JSON.parse(config.data);
        const item =
            requests.length === 1 ? { resourceId, error: { code: 503, reason: "materials_variant_not_ready", msg: "准备中" } } : { resourceId, access: { resourceId, url: "https://cdn.example.test/ready.mp4", delivery: "cdn", actualVariant: "playback" } };
        return { config, headers: {}, status: 200, statusText: "OK", data: { code: 0, data: { items: [item] } } };
    };
    const host = document.createElement("div");
    const root = createRoot(host);
    const noop = () => {};
    try {
        await act(async () =>
            root.render(
                <App>
                    <MediaResult
                        item={{ id: "new-video", role: "assistant", mode: "video", status: "done", content: "", createdAt: "2026-11-15", resultStorageKeys: ["resource:new-video"], resultUrls: ["/api/resources/new-video/file"] }}
                        onRetryFailure={noop}
                        onRefreshResult={noop}
                        onRecoverMedia={noop}
                        onCreateVariant={noop}
                        onContinueCanvas={noop}
                        openingCanvas={false}
                    />
                </App>,
            ),
        );
        await act(async () => {
            await Bun.sleep(100);
        });
        expect(requests).toEqual(["/resources/access"]);
        expect(host.textContent).toContain("正在准备播放");
        expect(host.querySelector("video")).toBeNull();
        expect(host.querySelector('button[aria-label="预览生成视频"]')).toBeNull();
        await act(async () => {
            await Bun.sleep(2300);
        });
        const video = host.querySelector("video")!;
        expect(video?.getAttribute("src")).toBe("https://cdn.example.test/ready.mp4");
        await act(async () => video.dispatchEvent(new Event("loadedmetadata")));
        expect(host.querySelector<HTMLButtonElement>('button[aria-label="预览生成视频"]')?.disabled).toBe(false);
        await act(async () => video.dispatchEvent(new Event("error")));
        expect(host.textContent).toContain("视频加载失败");
        expect(requests).toEqual(["/resources/access", "/resources/access"]);
        await act(async () => host.querySelector<HTMLButtonElement>('button[aria-label="视频加载失败，点击重新加载"]')!.click());
        await act(async () => {
            await Bun.sleep(100);
        });
        expect(host.querySelector("video")?.getAttribute("src")).toBe("https://cdn.example.test/ready.mp4");
        expect(requests).toEqual(["/resources/access", "/resources/access", "/resources/access"]);
    } finally {
        await act(async () => root.unmount());
    }
});

test("历史预览切走后停止等待，普通服务错误仍显示预览不可用", async () => {
    const calls: string[] = [];
    apiClient.defaults.adapter = async (config) => {
        const [{ resourceId }] = JSON.parse(config.data);
        calls.push(resourceId);
        const reason = resourceId === "still-preparing" ? "materials_variant_not_ready" : "material_access_unavailable";
        return { config, headers: {}, status: 200, statusText: "OK", data: { code: 0, data: { items: [{ resourceId, error: { code: 503, reason } }] } } };
    };
    const host = document.createElement("div");
    const root = createRoot(host);
    const render = (resourceId: string) => root.render(<TaskResultPreview item={{ key: resourceId, resourceId, kind: "video" }} variant="playback" alt="生成结果" controls />);
    try {
        await act(async () => render("still-preparing"));
        await act(async () => {
            await Bun.sleep(100);
        });
        expect(host.textContent).toContain("正在准备播放");
        await act(async () => render("unavailable"));
        await act(async () => {
            await Bun.sleep(2250);
        });
        expect(host.textContent).toContain("预览不可用");
        expect(host.querySelector("video")).toBeNull();
        expect(calls).toEqual(["still-preparing", "unavailable"]);
    } finally {
        await act(async () => root.unmount());
    }
});

test("固定结果按 Resource 转到画布，也可单独采用为创作素材", async () => {
    useUserStore.setState({ platformIdentity: true });
    const writes: string[] = [];
    apiClient.defaults.adapter = async (config) => {
        let data: unknown = { items: [] };
        let code = 0;
        if (config.url === "/resources/existing") {
            data = { resource: { id: "existing", provider: "platform", status: "ready", kind: "image", width: 640, height: 480 } };
        } else if (config.url === "/assets/platform-existing") {
            if (config.method === "put") {
                writes.push(JSON.parse(config.data).asset.data.storageKey);
            } else {
                code = 404;
            }
        } else if (config.url !== "/resources/access") {
            throw new Error(`Unexpected request ${config.method} ${config.url}`);
        }
        return { config, headers: {}, status: 200, statusText: "OK", data: { code, data } };
    };
    const host = document.createElement("div");
    document.body.append(host);
    const root = createRoot(host);
    const selections: unknown[] = [];
    const noop = () => {};
    const render = (fixed: boolean) =>
        root.render(
            <App>
                <MediaResult
                    item={{
                        id: "selected-message",
                        role: "assistant",
                        mode: "image",
                        status: "done",
                        content: "已有图片",
                        createdAt: "2026-11-09T00:00:00Z",
                        ...(fixed ? { resultStorageKeys: ["resource:existing"] } : { resultUrls: ["https://expired.test/image.png"] }),
                    }}
                    onRetryFailure={noop}
                    onRefreshResult={noop}
                    onRecoverMedia={noop}
                    onCreateVariant={noop}
                    onContinueCanvas={(selection) => selections.push(selection)}
                    openingCanvas={false}
                />
            </App>,
        );
    const button = (label: string) => [...host.querySelectorAll<HTMLButtonElement>("button")].find((b) => b.textContent?.trim() === label)!;
    try {
        await act(async () => render(true));
        expect(button("添加到画布").disabled).toBe(false);
        await act(async () => button("添加到画布").click());
        expect(selections).toEqual([{ assetIds: [], resourceIds: ["existing"] }]);
        expect(writes).toEqual([]);
        await act(async () => button("加入创作素材").click());
        expect(writes).toEqual(["resource:existing"]);
        await act(async () => render(false));
        expect(button("添加到画布").disabled).toBe(true);
        expect(button("加入创作素材").disabled).toBe(true);
    } finally {
        await act(async () => root.unmount());
        host.remove();
    }
});

test("结果下载按显示条目的固定 Resource 请求原件，拒绝时不下载预览", async () => {
    useUserStore.setState({ platformIdentity: true });
    clearResourceAccessCache();
    const requests: Array<{ resourceId: string; purpose: string; variant: string }> = [];
    const downloads: string[] = [];
    let rejectDownload = false;
    apiClient.defaults.adapter = async (config) => {
        if (config.url !== "/resources/access") throw new Error(`Unexpected request ${config.method} ${config.url}`);
        const payload = JSON.parse(config.data) as typeof requests;
        requests.push(...payload);
        const items = payload.map((request) => ({
            resourceId: request.resourceId,
            ...(request.resourceId === "missing-output" || (rejectDownload && request.purpose === "download")
                ? { error: { code: 403, msg: "Access denied" } }
                : {
                      access: {
                          resourceId: request.resourceId,
                          url: request.purpose === "download" ? "https://cdn.example.test/original.png" : "https://cdn.example.test/preview.webp",
                          requestedVariant: request.variant,
                          actualVariant: request.purpose === "download" ? "original" : "preview",
                          delivery: "cdn",
                      },
                  }),
        }));
        return { config, headers: {}, status: 200, statusText: "OK", data: { code: 0, data: { items } } };
    };
    const originalClick = dom.HTMLAnchorElement.prototype.click;
    dom.HTMLAnchorElement.prototype.click = function () {
        downloads.push(this.href);
    };
    const host = document.createElement("div");
    document.body.append(host);
    const root = createRoot(host);
    const noop = () => {};
    try {
        await act(async () => {
            root.render(
                <App>
                    <MediaResult
                        item={{
                            id: "download-message",
                            role: "assistant",
                            mode: "image",
                            status: "done",
                            content: "",
                            createdAt: "2026-11-17T00:00:00Z",
                            resultStorageKeys: ["resource:missing-output", "resource:ready-output"],
                            resultUrls: ["", "https://cdn.example.test/preview.webp"],
                        }}
                        onRetryFailure={noop}
                        onRefreshResult={noop}
                        onRecoverMedia={noop}
                        onCreateVariant={noop}
                        onContinueCanvas={noop}
                        openingCanvas={false}
                    />
                </App>,
            );
        });
        await act(async () => new Promise((resolve) => setTimeout(resolve, 80)));
        const download = [...host.querySelectorAll<HTMLElement>("button, a")].find((element) => element.textContent?.trim() === "下载");
        expect(download).toBeDefined();
        await act(async () => {
            download!.click();
            await new Promise((resolve) => setTimeout(resolve, 80));
        });
        expect(requests.filter((request) => request.purpose === "download")).toEqual([{ resourceId: "ready-output", purpose: "download", variant: "original" }]);
        expect(downloads).toEqual(["https://cdn.example.test/original.png"]);
        clearResourceAccessCache();
        rejectDownload = true;
        await act(async () => {
            download!.click();
            await new Promise((resolve) => setTimeout(resolve, 80));
        });
        expect(downloads).toEqual(["https://cdn.example.test/original.png"]);
    } finally {
        await act(async () => root.unmount());
        host.remove();
        dom.HTMLAnchorElement.prototype.click = originalClick;
        clearResourceAccessCache();
    }
});
