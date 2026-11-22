import { afterAll, afterEach, expect, test } from "bun:test";
import { Window } from "happy-dom";
import { catalogVariants } from "./platform-partial-catalog";
import type { NativeMediaVariant } from "@/lib/platform-native-media";
import type { MaterialCounts } from "@/lib/platform-media-supply";
import type { AiConfig } from "@/stores/use-config-store";
import type { PublicChannelModel } from "@/services/api/logical-models";
import type { CanvasNodeData } from "@/types/canvas";

// The wrapper starts a fresh process so React detects this DOM before caching input-event support.
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
const { MediaParameterControls } = await import("@/components/media-parameter-controls");
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
function configFor(variant: NativeMediaVariant, capability: "image" | "video" = "image", variants = [variant]): AiConfig {
    const model = {
        id: "fixture",
        modelKey: "fixture",
        displayName: "Fixture",
        capability,
        available: true,

        platform: { model_type: capability, media_generation_profile: { gateway_protocol: variant.gateway_protocol, native_request_variant: variant.native_request_variant, default_variant: variant.id, variants } },
    } as unknown as PublicChannelModel;
    const channels = systemChannelModelChannels([{ id: "oceanway-platform", name: "OceanWay", displayName: "OceanWay", models: [model] }]);
    return { ...defaultConfig, channels, model: "oceanway-platform::fixture", imageModel: "oceanway-platform::fixture", nativeVariantId: variant.id, nativeParameters: "{}" };
}
let host: HTMLDivElement;
let root: ReturnType<typeof createRoot> | undefined;
let current: AiConfig;
async function renderConfig(initial: AiConfig, dock?: { variant: NativeMediaVariant; variants?: NativeMediaVariant[]; imageCount: number; mode?: "image" | "video"; canvas?: boolean }, references?: MaterialCounts) {
    current = initial;
    function Form() {
        const [config, setConfig] = useState(initial);
        function change(key: "nativeParameters" | "nativeVariantId", value: string) {
            setConfig((previous) => {
                current = { ...previous, [key]: value };
                return current;
            });
        }
        if (dock) return <MediaParameterControls mode={dock.mode ?? "video"} config={config} variants={dock.variants ?? [dock.variant]} references={{ reference_image: dock.imageCount }} onChange={change} canvas={dock.canvas} />;
        return <PlatformNativeMediaSettings config={config} references={references} onChange={change} />;
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
async function openDock(group: string) {
    const trigger = host.querySelector<HTMLButtonElement>(`button[aria-label^="${group}："]`)!;
    await act(async () => trigger.click());
    expect(trigger.getAttribute("aria-expanded")).toBe("true");
    const menu = document.querySelector(`.studio-video-menu[aria-label="${group}"]`)!;
    return (text: string) => {
        const choice = [...menu.querySelectorAll("button")].find((button) => button.textContent === text);
        expect(choice).toBeDefined();
        return choice!;
    };
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

test.each(["settings", "create", "canvas"])("%s renders independent resolution and ratio presets with editable output counts", async (surface) => {
    const variant: NativeMediaVariant = {
        ...image,
        defaults: { "/resolution": "1k", "/aspect_ratio": "auto", "/n": 1 },
        workbench_parameters: ["/resolution", "/aspect_ratio", "/n"],
        specifications_schema: { properties: { "/resolution": { enum: ["1k", "2k"] }, "/aspect_ratio": { enum: ["auto", "16:9"] }, "/n": { type: "integer", minimum: 1, maximum: 10 } } },
        native_request_variant: {
            ...image.native_request_variant,
            output_size: undefined,
            size_presets: [
                { id: "1k-auto", resolution: "1k", aspect_ratio: "auto", values: { resolution: "1k", aspect_ratio: "auto" } },
                { id: "2k-auto", resolution: "2k", aspect_ratio: "auto", values: { resolution: "2k", aspect_ratio: "auto" } },
                { id: "2k-wide", resolution: "2k", aspect_ratio: "16:9", values: { resolution: "2k", aspect_ratio: "16:9" } },
                { id: "withdrawn", resolution: "4k", aspect_ratio: "1:1", values: { unpublished: "4k" } },
            ],
        },
    };
    const dock = surface === "settings" ? undefined : { variant, mode: "image" as const, imageCount: 0, canvas: surface === "canvas" };
    const base = configFor(variant);
    await renderConfig(base, dock);
    expect(host.textContent).not.toContain("参数无效");
    expect(host.textContent).toContain("1k");
    if (surface === "settings") {
        await select("分辨率", "2k");
        await select("宽高比", "16:9");
        await type("/n", "10");
    } else {
        const size = await openDock("尺寸与分辨率");
        await act(async () => size("2k").click());
        await act(async () => size("16:9").click());
        expect(size("自动").disabled).toBe(false);
        await act(async () => size("自动").click());
        expect(JSON.parse(current.nativeParameters!)["/aspect_ratio"]).toBe("auto");
        await act(async () => size("16:9").click());
        const counts = await openDock("张数");
        await act(async () => counts("10 张").click());
    }
    const target = { "/resolution": "2k", "/aspect_ratio": "16:9", "/n": 10 };
    expect(backendProviderConfig(current, "image").nativeParameters).toEqual(target);
    await unmount();
    await renderConfig({ ...base, nativeParameters: JSON.stringify({ ...target, "/n": 11 }) }, dock);
    expect(() => backendProviderConfig(current, "image")).toThrow();
    if (surface === "settings") await type("/n", "2");
    else {
        const counts = await openDock("张数");
        await act(async () => counts("2 张").click());
    }
    expect(backendProviderConfig(current, "image").nativeParameters).toEqual({ ...target, "/n": 2 });
});

test("real image settings select exact tuples, reject unaligned custom sizes, and reset the local custom mode", async () => {
    await renderConfig(configFor(image, "image", [image, { ...image, id: "edit", gateway_protocol: "openai.image.edit" }]));
    expect(host.querySelector('[aria-label="生成模式"], [aria-label="请求协议"]')).toBeNull();
    expect(host.querySelector('[aria-label="/size"]')).toBeNull();
    await select("分辨率", "2K 级");
    expect(JSON.parse(current.nativeParameters!)["/size"]).toBe("2048x2048");
    await select("宽高比", "16:9");
    expect(JSON.parse(current.nativeParameters!)).toEqual({ "/size": "2560x1440" });
    expect(host.textContent).toContain("2560 × 1440 px");
    await select("尺寸方式", "自定义尺寸");
    await type("自定义宽度", "1025");
    expect(host.querySelector('[role="alert"]')?.textContent).toContain("16 的整数倍");
    expect(JSON.parse(current.nativeParameters!)["/size"]).toBe("1025x1440");
    expect(() => backendProviderConfig(current, "image")).toThrow("16 的整数倍");
    await act(async () => [...host.querySelectorAll("button")].find((button) => button.textContent === "恢复模型默认值")!.click());
    expect(host.querySelector('[aria-label="自定义宽度"]')).toBeNull();
    expect(host.textContent).toContain("1024 × 1024 px");
    expect(JSON.parse(current.nativeParameters!)).toEqual({});
    expect(backendProviderConfig(current, "image").nativeParameters).toEqual({ "/size": "1024x1024" });
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
    expect(backendProviderConfig(current, "image").nativeParameters).toEqual({});
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
    await select("分辨率", "2K");
    expect(JSON.parse(current.nativeParameters!)).toEqual({ "/generationConfig/imageConfig/imageSize": "2K", "/generationConfig/imageConfig/aspectRatio": "16:9" });
    expect(host.querySelector('[role="alert"]')?.textContent).toContain("当前比例与 imageSize 组合没有可用供给");
    await select("宽高比", "1:1");
    expect(backendProviderConfig(current, "image").nativeParameters).toEqual({ "/generationConfig/imageConfig/imageSize": "2K", "/generationConfig/imageConfig/aspectRatio": "1:1" });
});

test("the narrow image panel lets a resolution change select a published pixel tuple even when the old ratio is absent", async () => {
    const variant = { ...image, native_request_variant: { ...image.native_request_variant, size_presets: image.native_request_variant.size_presets!.filter((preset) => preset.id !== "square-2k") } };
    await renderConfig(configFor(variant));
    const axes = [...host.querySelectorAll('[role="combobox"]')].map((control) => control.getAttribute("aria-label")).filter((label) => ["分辨率", "宽高比"].includes(label!));
    expect(axes).toEqual(["分辨率", "宽高比"]);
    expect(selectedText("宽高比")).toContain("1:1");
    await select("分辨率", "2K 级");
    expect(selectedText("宽高比")).toContain("16:9");
    expect(JSON.parse(current.nativeParameters!)).toEqual({ "/size": "2560x1440" });
    expect(backendProviderConfig(current, "image").nativeParameters).toEqual({ "/size": "2560x1440" });
});

test.each(["before/", ""])("both image controls show the same effective dimensions as an empty draft in the %spartial product detail", async (prefix) => {
    const [variant] = catalogVariants(`${prefix}studio-detail-partial-1`);
    const config = configFor(variant);
    await renderConfig(config);
    const width = host.querySelector<HTMLInputElement>('[aria-label="自定义宽度"]');
    if (width) expect(width.value).toBe("1536");
    else expect(host.textContent).toContain("1536 × 1024 px");
    expect(host.textContent).not.toContain("1K");
    expect(host.querySelector('[role="alert"]')).toBeNull();
    expect(backendProviderConfig(current, "image").nativeParameters).toEqual({ "/size": "1536x1024" });
    await unmount();
    await renderConfig(config, { variant, imageCount: 0, mode: "image" });
    expect(host.querySelector('[aria-label^="尺寸与分辨率："]')?.textContent).toContain("1536");
    await openDock("尺寸与分辨率");
    expect(document.querySelector<HTMLInputElement>('input[aria-label="宽度"]')?.value).toBe("1536");
    expect(document.querySelector(".studio-video-menu")?.textContent).not.toContain("1K");
    expect(current.nativeParameters).toBe("{}");
});

test("the narrow image panel uses the current materials to explain pixel limits and the availability of auto", async () => {
    const low = { ...image.native_request_variant.output_size!, allow_auto: false, max_pixels: 4194304 };
    const high = { ...low, allow_auto: true, max_pixels: 8294400 };
    const slot = { id: "images", pointer: "/image", role: "reference_image", transport: "image-reference-array", min_items: 1, max_items: 16 };
    const edit: NativeMediaVariant = {
        ...image,
        gateway_protocol: "openai.image.edit",
        native_request_variant: {
            ...image.native_request_variant,
            protocol: "openai.image.edit",
            path: "/v1/images/edits",
            output_size: high,
            native_reference_inputs: [slot],
            supply_alternatives: [
                { request_schema: { required: ["size"] }, native_reference_inputs: [slot], output_size: low },
                { request_schema: { required: ["size"] }, native_reference_inputs: [{ ...slot, max_items: 1 }], output_size: high },
            ],
        },
    };
    const original = { ...configFor(edit), nativeParameters: '{"/size":"3072x2048"}' };
    await renderConfig(original, undefined, { reference_image: 16 });
    expect(host.textContent).toContain(`单张最多 ${(4194304).toLocaleString()} 像素`);
    expect(host.textContent).not.toContain((8294400).toLocaleString());
    expect(host.querySelector('[role="alert"]')?.textContent).toContain((6291456).toLocaleString());
    await select("尺寸方式", "自动尺寸");
    expect(current.nativeParameters).toBe(original.nativeParameters);
    await type("自定义宽度", "2048");
    expect(host.querySelector('[role="alert"]')).toBeNull();
    expect(backendProviderConfig(current, "image", { reference_image: 16 }).nativeParameters).toEqual({ "/size": "2048x2048" });
    await unmount();
    await renderConfig(original, undefined, { reference_image: 1 });
    expect(host.textContent).toContain(`单张最多 ${(8294400).toLocaleString()} 像素`);
    expect(host.querySelector('[role="alert"]')).toBeNull();
    await select("尺寸方式", "自动尺寸");
    expect(backendProviderConfig(current, "image", { reference_image: 1 }).nativeParameters).toEqual({ "/size": "auto" });
});

test("optional booleans can go from omitted to on, off and omitted, and fixed values stay truthful", async () => {
    const video: NativeMediaVariant = {
        id: "video",
        gateway_protocol: "modelark.video",
        defaults: { "/generate_audio": true, "/watermark": false, "/seed": 3 },
        specifications_schema: { required: ["/watermark"], properties: { "/generate_audio": { type: "boolean" }, "/watermark": { const: false }, "/profile": { const: "official" }, "/seed": { type: "integer" } } },
        native_request_variant: { protocol: "modelark.video", path: "/video", modes: ["async"], request_schema: {}, native_reference_inputs: [] },
    };
    const initial = { ...configFor(video, "video"), nativeParameters: '{"/seed":0}' };
    await renderConfig(initial);
    expect(selectedText("/generate_audio")).toContain("模型默认");
    const fixed = host.querySelector<HTMLInputElement>('input[aria-label="/watermark"]')!;
    expect(fixed.readOnly).toBe(true);
    expect(fixed.value).toBe("关");
    expect(host.querySelector<HTMLInputElement>('input[aria-label="/profile"]')?.value).toBe("official");
    expect(backendProviderConfig(current, "video").nativeParameters).toEqual({ "/seed": 0, "/watermark": false });
    for (const [label, value] of [
        ["开", true],
        ["关", false],
    ] as const) {
        await select("/generate_audio", label);
        expect(backendProviderConfig(current, "video").nativeParameters).toEqual({ "/seed": 0, "/watermark": false, "/generate_audio": value });
    }
    await select("/generate_audio", "模型默认");
    expect(JSON.parse(current.nativeParameters!)).toEqual({ "/seed": 0 });
    expect(backendProviderConfig(current, "video").nativeParameters).toEqual({ "/seed": 0, "/watermark": false });
    await unmount();
    await renderConfig({ ...initial, nativeParameters: '{"/seed":0,"/watermark":true}' });
    expect(host.querySelector('[role="alert"]')?.textContent).toContain("/watermark");
    await act(async () => [...host.querySelectorAll("button")].find((button) => button.textContent === "使用固定值")!.click());
    expect(host.querySelector('[role="alert"]')).toBeNull();
    expect(JSON.parse(current.nativeParameters!)).toEqual({ "/seed": 0 });
    expect(backendProviderConfig(current, "video").nativeParameters).toEqual({ "/seed": 0, "/watermark": false });
});

test("video presets retain the normal video parameters and do not show an image size widget", async () => {
    const video = { ...image, gateway_protocol: "modelark.video", native_request_variant: { ...image.native_request_variant, protocol: "modelark.video" } };
    await renderConfig(configFor(video, "video"));
    expect(host.textContent).not.toContain("输出尺寸");
    expect(host.querySelector('[aria-label="/size"]')).not.toBeNull();
});

test.each([false, true])("video controls show resolution before reference roles, duration and ratio (canvas=%p)", async (canvas) => {
    const base: NativeMediaVariant = {
        id: "text",
        gateway_protocol: "modelark.video",
        defaults: { "/resolution": "720p", "/duration": 5, "/ratio": "16:9" },
        specifications_schema: { properties: { "/ratio": { enum: ["16:9", "1:1"] }, "/duration": { enum: [5, 10] }, "/resolution": { enum: ["720p", "1080p"] } } },
        workbench_parameters: ["/ratio", "/duration", "/resolution"],
        native_request_variant: { protocol: "modelark.video", path: "/video", modes: ["async"], request_schema: {}, native_reference_inputs: [] },
    };
    const roles = (id: string, inputs: string[]): NativeMediaVariant => ({
        ...base,
        id,
        native_request_variant: { ...base.native_request_variant, native_reference_inputs: inputs.map((role) => ({ role, id: role, pointer: `/${role}`, transport: "url", max_items: 1 })) },
    });
    const ordinary = roles("ordinary", ["reference_image"]);
    const frames = roles("frames", ["first_frame", "last_frame"]);
    const variants = [base, ordinary, frames];
    await renderConfig(configFor(base, "video", variants), { variant: base, variants, imageCount: 0, canvas });
    const groups = [...host.querySelectorAll(".studio-video-trigger")].map((control) => control.getAttribute("aria-label")?.split("：")[0]);
    expect(groups).toEqual(["尺寸与分辨率", "生成模式", "时长", "比例"]);
    const options = await openDock("生成模式");
    expect(document.querySelector('.studio-video-menu[aria-label="生成模式"]')?.textContent).not.toContain("文生视频");
    expect(options("图片参考")).toBeDefined();
    await act(async () => options("首尾帧").click());
    expect(current.nativeVariantId).toBe(frames.id);
});

test("the video production controls retain a conflicting duration after a resolution change and offer only that tier's durations", async () => {
    const ref = { id: "images", pointer: "/images", role: "reference_image", transport: "url-array", max_items: 3 };
    const variant: NativeMediaVariant = {
        id: "video",
        gateway_protocol: "modelark.video",
        defaults: { "/resolution": "720p", "/duration": 5 },
        specifications_schema: { properties: { "/resolution": { enum: ["720p", "1080p"] }, "/duration": { enum: [4, 5, 6, 8, 10] } } },
        workbench_parameters: ["/resolution", "/duration"],
        native_request_variant: {
            path: "/video",
            protocol: "modelark.video",
            modes: ["async"],
            request_schema: {},
            native_reference_inputs: [ref],
            supply_alternatives: [
                { request_schema: { properties: { resolution: { const: "720p" }, duration: { enum: [4, 5, 6, 8, 10] } } }, native_reference_inputs: [ref] },
                { request_schema: { properties: { resolution: { const: "1080p" }, duration: { enum: [4, 6, 8, 10] } } }, native_reference_inputs: [{ ...ref, max_items: 1 }] },
            ],
        },
    };
    const config = { ...configFor(variant, "video"), nativeParameters: '{"/resolution":"720p","/duration":5}' };
    await renderConfig(config, { variant, imageCount: 3 });
    const sizes = await openDock("尺寸与分辨率");
    const hd = sizes("1080p");
    expect(hd.disabled).toBe(false);
    await act(async () => hd.click());
    expect(JSON.parse(current.nativeParameters!)).toEqual({ "/resolution": "1080p", "/duration": 5 });
    expect(() => backendProviderConfig(current, "video", { reference_image: 3 })).toThrow("最多支持 1 张参考图");
    const blocked = await openDock("时长");
    for (const time of [4, 5, 6, 8, 10]) expect(blocked(`${time}秒`).disabled).toBe(true);
    const saved = current;
    await unmount();
    await renderConfig(saved, { variant, imageCount: 1 });
    const durations = await openDock("时长");
    expect(durations("5秒").disabled).toBe(true);
    for (const time of [4, 6, 8, 10]) expect(durations(`${time}秒`).disabled).toBe(false);
    await act(async () => durations("6秒").click());
    expect(JSON.parse(current.nativeParameters!)).toEqual({ "/resolution": "1080p", "/duration": 6 });
    expect(backendProviderConfig(current, "video", { reference_image: 1 }).nativeParameters).toEqual({ "/resolution": "1080p", "/duration": 6 });
});

test.each(["settings", "create", "canvas"])("%s keeps withdrawn Seedance 480p in the old draft until the user selects a supported resolution", async (surface) => {
    const pointer = "/metadata/resolution";
    const variant: NativeMediaVariant = {
        id: "seedance-2.0-ch01",
        gateway_protocol: "newapi.video",
        defaults: { [pointer]: "720p", "/duration": 5 },
        workbench_parameters: [pointer, "/duration"],
        specifications_schema: { required: [pointer, "/duration"], properties: { [pointer]: { enum: ["1080p", "480p", "4k", "720p"] }, "/duration": { enum: [5, 10] } } },
        native_request_variant: {
            protocol: "newapi.video",
            path: "/v1/video/generations",
            modes: ["async"],
            request_schema: {},
            native_reference_inputs: [],
            supply_alternatives: ["1080p", "4k", "720p"].map((resolution) => ({ request_schema: { properties: { metadata: { properties: { resolution: { enum: [resolution] } } }, duration: { enum: [5, 10] } } }, native_reference_inputs: [] })),
        },
    };
    const base = configFor(variant, "video");
    const dock = surface === "settings" ? undefined : { variant, imageCount: 0, canvas: surface === "canvas" };
    await renderConfig(base, dock);
    expect(host.textContent).toContain("720p");
    expect(backendProviderConfig(current, "video").nativeParameters).toEqual({ [pointer]: "720p", "/duration": 5 });
    await unmount();
    await renderConfig({ ...base, nativeParameters: JSON.stringify({ [pointer]: "480p", "/duration": 99 }) }, dock);
    expect(host.textContent).toContain("480p");
    expect(() => backendProviderConfig(current, "video")).toThrow();
    if (surface === "settings") {
        const input = host.querySelector<HTMLElement>(`[role="combobox"][aria-label="${pointer}"]`)!;
        await act(async () => input.dispatchEvent(new MouseEvent("mousedown", { bubbles: true })));
        const option = (text: string) => [...document.querySelectorAll<HTMLElement>(".ant-select-item-option")].find((node) => node.textContent === text)!;
        expect(option("480p").getAttribute("aria-disabled")).toBe("true");
        await act(async () => option("720p").click());
        await select("/duration", "5");
    } else {
        const resolution = await openDock("尺寸与分辨率");
        expect(resolution("480p").disabled).toBe(true);
        expect(resolution("720p").disabled).toBe(false);
        await act(async () => resolution("720p").click());
        const duration = await openDock("时长");
        await act(async () => duration("5秒").click());
    }
    expect(backendProviderConfig(current, "video").nativeParameters).toEqual({ [pointer]: "720p", "/duration": 5 });
});
