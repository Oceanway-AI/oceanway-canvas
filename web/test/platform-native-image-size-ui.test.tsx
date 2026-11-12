import { afterAll, afterEach, expect, test } from "bun:test";
import { Window } from "happy-dom";
import type { NativeMediaVariant } from "@/lib/platform-native-media";
import type { AiConfig } from "@/stores/use-config-store";
import type { PublicChannelModel } from "@/services/api/logical-models";
import type { CanvasNodeData } from "@/types/canvas";

const dom = new Window({ url: "https://studio.example.test" });
const globals = new Map<string, PropertyDescriptor | undefined>();
for (const [key, value] of Object.entries({
    window: dom,
    document: dom.document,
    location: dom.location,
    navigator: dom.navigator,
    HTMLElement: dom.HTMLElement,
    HTMLInputElement: dom.HTMLInputElement,
    Element: dom.Element,
    Node: dom.Node,
    SVGElement: dom.SVGElement,
    ShadowRoot: dom.ShadowRoot,
    Event: dom.Event,
    MouseEvent: dom.MouseEvent,
    MutationObserver: dom.MutationObserver,
    ResizeObserver: dom.ResizeObserver,
    getComputedStyle: dom.getComputedStyle.bind(dom),
    IS_REACT_ACT_ENVIRONMENT: true,
})) {
    globals.set(key, Object.getOwnPropertyDescriptor(globalThis, key));
    Object.defineProperty(globalThis, key, { configurable: true, writable: true, value });
}
const { act, useState } = await import("react");
const { createRoot } = await import("react-dom/client");
const { PlatformNativeMediaSettings } = await import("@/components/platform-native-media-settings");
const { defaultConfig } = await import("@/stores/use-config-store");
const { systemChannelModelChannels } = await import("@/lib/user-session");
const { backendProviderConfig } = await import("@/services/api/generation-task");
const { buildNodeConfig } = await import("@/components/canvas/canvas-node-prompt-config");
const { buildGenerationConfig } = await import("@/lib/canvas/canvas-project-generation");
const { specFromConfig, generationSpecMetadata } = await import("@/lib/canvas/generation-contract");
const { CanvasNodeType } = await import("@/types/canvas");
const { apiClient } = await import("@/services/api/request");
const previousAdapter = apiClient.defaults.adapter;
apiClient.defaults.adapter = async (config) => {
    throw new Error(`Unexpected request: ${config.method} ${config.url}`);
};

const image: NativeMediaVariant = {
    id: "image",
    gateway_protocol: "openai.image.generate",
    defaults: { "/size": "1024x1024", "/n": 2 },
    specifications_schema: { properties: { "/size": { type: "string" }, "/n": { type: "integer" } } },
    native_request_variant: {
        protocol: "openai.image.generate",
        path: "/v1/images/generations",
        modes: ["sync"],
        request_schema: {},
        native_reference_inputs: [],
        size_presets: [
            { id: "square-1k", resolution: "1K", aspect_ratio: "1:1", width: 1024, height: 1024, values: { size: "1024x1024" } },
            { id: "square-2k", resolution: "2K", aspect_ratio: "1:1", width: 2048, height: 2048, values: { size: "2048x2048" } },
            { id: "wide-2k", resolution: "2K", aspect_ratio: "16:9", width: 2560, height: 1440, values: { size: "2560x1440" } },
        ],
        output_size: { allow_custom: true, allow_auto: true, multiple_of: 16, min_width: 16, max_width: 3840, min_height: 16, max_height: 3840, min_pixels: 655360, max_pixels: 4194304, min_aspect_ratio: 1 / 3, max_aspect_ratio: 3 },
    },
};
function configFor(variant: NativeMediaVariant, capability: "image" | "video" = "image"): AiConfig {
    const model = {
        id: "fixture",
        modelKey: "fixture",
        displayName: "Fixture",
        capability,
        available: true,
        priceTiers: [],
        platform: { model_type: capability, media_generation_profile: { gateway_protocol: variant.gateway_protocol, native_request_variant: variant.native_request_variant, default_variant: variant.id, variants: [variant] } },
    } as unknown as PublicChannelModel;
    const channels = systemChannelModelChannels([{ id: "oceanway-platform", name: "OceanWay", displayName: "OceanWay", models: [model] }]);
    return { ...defaultConfig, channels, model: "oceanway-platform::fixture", imageModel: "oceanway-platform::fixture", nativeVariantId: variant.id, nativeParameters: "{}" };
}
let host: HTMLDivElement;
let root: ReturnType<typeof createRoot> | undefined;
let current: AiConfig;
async function renderConfig(initial: AiConfig) {
    current = initial;
    function Form() {
        const [config, setConfig] = useState(initial);
        return (
            <PlatformNativeMediaSettings
                config={config}
                onChange={(key, value) =>
                    setConfig((previous) => {
                        current = { ...previous, [key]: value };
                        return current;
                    })
                }
            />
        );
    }
    host = document.createElement("div");
    document.body.append(host);
    root = createRoot(host);
    await act(async () => root!.render(<Form />));
}
async function unmount() {
    await act(async () => root?.unmount());
    root = undefined;
    host?.remove();
}
async function select(label: string, text: string) {
    const input = host.querySelector<HTMLElement>(`[role="combobox"][aria-label="${label}"]`);
    expect(input).not.toBeNull();
    await act(async () => input!.dispatchEvent(new MouseEvent("mousedown", { bubbles: true })));
    const option = [...document.querySelectorAll<HTMLElement>(".ant-select-item-option")].find((item) => item.textContent === text);
    expect(option).not.toBeUndefined();
    await act(async () => option!.click());
}
async function type(label: string, value: string) {
    const input = host.querySelector<HTMLInputElement>(`input[aria-label="${label}"]`)!;
    expect(input).not.toBeNull();
    await act(async () => {
        Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!.call(input, value);
        input.dispatchEvent(new Event("input", { bubbles: true }));
    });
}
function selectedText(label: string) {
    return host.querySelector(`[aria-label="${label}"]`)?.closest(".ant-select")?.textContent;
}
afterEach(unmount);
afterAll(() => {
    apiClient.defaults.adapter = previousAdapter;
    dom.happyDOM.abort();
    for (const [key, descriptor] of globals) {
        if (descriptor) Object.defineProperty(globalThis, key, descriptor);
        else Reflect.deleteProperty(globalThis, key);
    }
});

test("real image settings select exact tuples, reject unaligned custom sizes, and reset the local custom mode", async () => {
    await renderConfig(configFor(image));
    expect(host.querySelector('[aria-label="/size"]')).toBeNull();
    await select("分辨率", "2K 级");
    expect(JSON.parse(current.nativeParameters!)["/size"]).toBe("2048x2048");
    await select("宽高比", "16:9");
    expect(JSON.parse(current.nativeParameters!)).toEqual({ "/size": "2560x1440", "/n": 2 });
    expect(host.textContent).toContain("2560 × 1440 px");
    await select("尺寸方式", "自定义尺寸");
    await type("自定义宽度", "1025");
    expect(host.querySelector('[role="alert"]')?.textContent).toContain("16 的整数倍");
    expect(JSON.parse(current.nativeParameters!)["/size"]).toBe("1025x1440");
    expect(() => backendProviderConfig(current, "image")).toThrow("16 的整数倍");
    await act(async () => [...host.querySelectorAll("button")].find((button) => button.textContent === "恢复模型默认值")!.click());
    expect(host.querySelector('[aria-label="自定义宽度"]')).toBeNull();
    expect(host.textContent).toContain("1024 × 1024 px");
    expect(backendProviderConfig(current, "image").nativeParameters).toEqual(image.defaults);
});

test("canvas save and reopen retain the same exact custom dimensions in the editor and executor", async () => {
    const config = configFor(image);
    await renderConfig(config);
    await select("尺寸方式", "自定义尺寸");
    await type("自定义宽度", "1536");
    await type("自定义高度", "1024");
    const submitted = backendProviderConfig(current, "image");
    const spec = specFromConfig("image", "A quiet ocean", current, []);
    const saved = { id: "image-node", type: CanvasNodeType.Image, position: { x: 0, y: 0 }, metadata: generationSpecMetadata(spec) } as CanvasNodeData;
    const restored = JSON.parse(JSON.stringify(saved)) as CanvasNodeData;
    const panel = buildNodeConfig(config, restored, "image", { capability: "image" });
    const executor = buildGenerationConfig(config, restored, "image", { capability: "image" });
    await unmount();
    await renderConfig(panel);
    expect(host.querySelector<HTMLInputElement>('[aria-label="自定义宽度"]')?.value).toBe("1536");
    expect(host.querySelector<HTMLInputElement>('[aria-label="自定义高度"]')?.value).toBe("1024");
    expect(backendProviderConfig(panel, "image")).toEqual(submitted);
    expect(backendProviderConfig(executor, "image")).toEqual(submitted);
});

test("Gemini displays its native default without inventing a ratio, links valid tuples, and sends no size", async () => {
    const gemini: NativeMediaVariant = {
        ...image,
        gateway_protocol: "gemini.image",
        defaults: { "/generationConfig/imageConfig/imageSize": "2K" },
        specifications_schema: { properties: { "/generationConfig/imageConfig/imageSize": { type: "string" }, "/generationConfig/imageConfig/aspectRatio": { type: "string" } } },
        native_request_variant: {
            ...image.native_request_variant,
            protocol: "gemini.image",
            output_size: undefined,
            size_presets: [
                { id: "square", resolution: "2K", aspect_ratio: "1:1", values: { generationConfig: { imageConfig: { imageSize: "2K", aspectRatio: "1:1" } } } },
                { id: "wide", resolution: "4K", aspect_ratio: "16:9", values: { generationConfig: { imageConfig: { imageSize: "4K", aspectRatio: "16:9" } } } },
            ],
        },
    };
    await renderConfig(configFor(gemini));
    expect(current.nativeParameters).toBe("{}");
    expect(selectedText("分辨率")).toContain("2K");
    expect(selectedText("宽高比")).toContain("选择比例");
    expect(host.querySelector('[aria-label="尺寸方式"]')).toBeNull();
    expect(host.querySelector('[aria-label="/generationConfig/imageConfig/imageSize"]')).toBeNull();
    await select("宽高比", "16:9");
    expect(selectedText("分辨率")).toContain("4K");
    const body = backendProviderConfig(current, "image").nativeParameters;
    expect(body).toEqual({ "/generationConfig/imageConfig/imageSize": "4K", "/generationConfig/imageConfig/aspectRatio": "16:9" });
    expect(body).not.toHaveProperty("/size");
    expect(() => backendProviderConfig({ ...current, nativeParameters: JSON.stringify({ ...body, "/size": "1024x1024" }) }, "image")).toThrow("imageSize");
});

test("video presets retain the normal video parameters and do not show an image size widget", async () => {
    const video = { ...image, gateway_protocol: "modelark.video", native_request_variant: { ...image.native_request_variant, protocol: "modelark.video" } };
    await renderConfig(configFor(video, "video"));
    expect(host.textContent).not.toContain("输出尺寸");
    expect(host.querySelector('[aria-label="/size"]')).not.toBeNull();
});
