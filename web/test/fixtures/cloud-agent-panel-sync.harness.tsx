import { afterAll, expect, mock, test } from "bun:test";
import { Window } from "happy-dom";
import type { AgentEvent, AgentRun } from "@/services/api/agent";
import type { CloudAgentConversation, CloudAgentPendingSubmission } from "@/services/cloud-agent-conversations";

const dom = new Window({ url: "http://localhost/" });
for (const [key, value] of Object.entries({
    window: dom,
    document: dom.document,
    navigator: dom.navigator,
    localStorage: dom.localStorage,
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
const React = await import("react");
const { act } = React;
const { createRoot } = await import("react-dom/client");
const agent = await import("@/services/api/agent");
const history = await import("@/services/cloud-agent-conversations");
const awaitImportHistory = await import("@/services/cloud-agent-history");
const skillsApi = await import("@/services/api/skills");
const { setActiveUserScope } = await import("@/lib/user-scope");
const { useUserStore } = await import("@/stores/use-user-store");
const { useGlobalAgentStore } = await import("@/stores/use-global-agent-store");
const { createModelChannel, defaultConfig, normalizeConfigSnapshot, useConfigStore } = await import("@/stores/use-config-store");
const { ApiError } = await import("@/services/api/request");

let local: CloudAgentConversation[] = [];
let remote: CloudAgentConversation[] = [];
let activeId = "";
let pending: CloudAgentPendingSubmission | null = null;
let refreshOverride: (() => Promise<CloudAgentConversation[]>) | undefined;
const subscriptions: Array<{ id: string; after: number; event: (event: AgentEvent) => void; connection: (status: string) => void; closed: boolean }> = [];
const sent: string[] = [];
const sendInputs: Array<Record<string, any>> = [];
const createInputs: Array<Record<string, any>> = [];
let sendFailure: Error | undefined;
let profileOffline = false;
let draftProfile: ((canvasId?: string) => Promise<void>) | undefined;
let prepareDraft: ((canvasId: string) => void) | undefined;
let capabilitiesOverride: (() => Promise<unknown>) | undefined;
let sendOverride: (() => Promise<{ run: AgentRun }>) | undefined;
let createOverride: (() => Promise<{ run: AgentRun }>) | undefined;
let mediaPrepare: (() => Promise<string>) | undefined;
let mediaDecision: ((decision: string, quoteId?: string, enableAuto?: boolean) => void) | undefined;
const refreshRequests: Array<{ observedConversationIds?: string[]; canvasId?: string }> = [];
let generated = 0;
let composer: Record<string, any>;
let header: Record<string, any>;
let conversationUI: Record<string, any>;
let historyUI: Record<string, any>;
let controls: Record<string, any>;
const silent = () => {};
mock.module("file-saver", () => ({ saveAs: silent }));
const MotionBox = ({ children, className, style }: any) => (
    <div className={className} style={style}>
        {children}
    </div>
);
mock.module("motion/react", () => ({ AnimatePresence: ({ children }: any) => children, motion: { aside: MotionBox, div: MotionBox, li: MotionBox }, useReducedMotion: () => true }));
mock.module("antd", () => ({
    App: { useApp: () => ({ message: { info: silent, error: silent, warning: silent, success: silent } }) },
    Dropdown: ({ children }: any) => children,
    Button: ({ children, loading: _loading, icon, size: _size, ...props }: any) => (
        <button {...props}>
            {icon}
            {children}
        </button>
    ),
}));
mock.module("@/services/api/agent", () => ({
    ...agent,
    getAgentProfile: async ({ canvasId }: { canvasId?: string }) => {
        if (profileOffline) throw new ApiError("Network Error", { retryable: true });
        await draftProfile?.(canvasId);
        return { revision: "profile", hash: "hash", layers: [] };
    },
    getAgentCapabilities: async () => (capabilitiesOverride ? capabilitiesOverride() : { permissionModes: ["request_approval"], skills: true }),
    subscribeAgentEvents: (id: string, event: (event: AgentEvent) => void, options: any) => {
        const subscription = { id, after: options.after, event, connection: options.onConnectionChange, closed: false };
        subscriptions.push(subscription);
        options.onConnectionChange("connected");
        return () => {
            subscription.closed = true;
        };
    },
    sendAgentMessage: async (id: string, input: any) => {
        sent.push(id);
        sendInputs.push(input);
        if (sendFailure) throw sendFailure;
        if (sendOverride) return sendOverride();
        return { run: { ...makeRun("submitted-child"), parentId: id, userPrompt: input.prompt, idempotencyKey: input.idempotencyKey } };
    },
    createAgentRun: async (input: any) => {
        generated++;
        createInputs.push(input);
        if (createOverride) return createOverride();
        throw new Error("不应新建独立轮次");
    },
    prepareAgentMediaApproval: async () => {
        if (mediaPrepare) return mediaPrepare();
        throw new Error("意外的媒体准备请求");
    },
    decideAgentApproval: async (_run: string, _approval: string, decision: string, _reason?: string, _signal?: AbortSignal, _settings?: unknown, quoteId?: string, enableAuto?: boolean) => {
        if (mediaDecision) return mediaDecision(decision, quoteId, enableAuto);
        generated++;
        throw new Error("测试禁止审批");
    },
}));
mock.module("@/services/cloud-agent-conversations", () => ({
    ...history,
    loadCloudAgentConversations: async (scope: string) => (scope === "sync-user" ? { version: 1, activeId, conversations: local } : { version: 1, activeId: null, conversations: [] }),
    saveCloudAgentConversations: async (id: string, conversations: CloudAgentConversation[], scope: string) => {
        if (scope !== "sync-user") return;
        activeId = id;
        local = conversations;
    },
    loadCloudAgentPendingSubmission: async (_id: string, scope: string) => (scope === "sync-user" ? pending : null),
    saveCloudAgentPendingSubmission: async (_id: string, value: CloudAgentPendingSubmission, scope: string) => {
        if (scope !== "sync-user") throw new Error("不能向其他账号保存原请求");
        pending = value;
    },
    clearCloudAgentPendingSubmission: async (_id: string, scope: string) => {
        if (scope !== "sync-user") throw new Error("不能清除其他账号原请求");
        pending = null;
    },
}));
mock.module("@/services/cloud-agent-history", () => ({
    ...awaitImportHistory,
    refreshCloudAgentConversations: async (options: any) => {
        refreshRequests.push(options);
        if (options.scope !== "sync-user") return [];
        return refreshOverride ? refreshOverride() : remote;
    },
    loadCloudAgentConversation: async (id: string) => remote.find((item) => item.serverConversationId === id)!,
}));
mock.module("@/services/api/skills", () => ({
    ...skillsApi,
    listSkillPresets: async () => ({ presets: [] }),
    listSkills: async () => ({ skills: [] }),
    listAddedSkills: async () => ({ skills: [] }),
    listSkillLibraryCategories: async () => ({ categories: [] }),
    addSkill: async () => {
        throw new Error("不应安装技能");
    },
}));
mock.module("@/services/user-data-sync", () => ({ prepareCanvasForCloudAction: async (id: string) => prepareDraft?.(id), applyAgentCanvasPatches: async () => {}, refreshCanvasAfterAgent: async () => {} }));
mock.module("@/components/skills/skill-curation-browser", () => ({ useSkillCuration: () => ({ curation: null }), curationQuery: () => ({}) }));
mock.module("@/components/canvas/canvas-cloud-agent-chat-ui", () => ({
    AGENT_SCENE_DEFS: [],
    parseCloudAgentFormAnswer: () => undefined,
    AgentChatComposer: (props: any) => {
        composer = props;
        return (
            <>
                {props.tray}
                {props.left}
                <textarea aria-label="草稿" value={props.prompt} readOnly />
            </>
        );
    },
    AgentPlanBar: () => null,
    AgentQuestionBar: () => null,
    AgentSceneCapsules: () => null,
}));
mock.module("@/components/canvas/canvas-cloud-agent-panel-parts", () => ({
    AgentLauncher: () => null,
    AgentContextRing: () => null,
    ComposerControls: (props: any) => {
        controls = props;
        return null;
    },
    AgentHeader: (props: any) => {
        header = props;
        return <div>{props.statusLabel}</div>;
    },
    AgentConversation: (props: any) => {
        conversationUI = props;
        return (
            <div data-testid="conversation">
                {props.messages.map((item: any) => (
                    <div key={item.id}>{item.text}</div>
                ))}
            </div>
        );
    },
    AgentHistory: (props: any) => {
        historyUI = props;
        return <div>历史</div>;
    },
}));
mock.module("@/components/canvas/canvas-agent-skill-library-modal", () => ({ CanvasAgentSkillLibraryModal: () => null }));
mock.module("@/components/skills/skill-recommendations-modal", () => ({ SkillRecommendationsModal: () => null }));
mock.module("@/components/canvas/canvas-cloud-agent-settings", () => ({ CanvasCloudAgentSettings: () => null, agentPermissionLabel: (mode: string) => mode }));
mock.module("@/components/canvas/canvas-cloud-agent-composer", () => ({ AgentContextChip: () => null }));
mock.module("@/services/cloud-agent-outbox", () => ({ loadCloudAgentOutbox: async () => null, saveCloudAgentOutbox: async () => {}, clearCloudAgentOutbox: async () => {} }));
mock.module("@/components/canvas/canvas-cloud-agent-connectors", () => ({ AgentConnectorsBar: () => null }));
mock.module("@/components/canvas/use-agent-panel-layout", () => ({ useAgentPanelLayout: () => ({ style: {}, pointerHandlers: {}, reset: silent, onResizeKeyDown: silent }) }));

const { CanvasCloudAgentPanel } = await import("@/components/canvas/canvas-cloud-agent-panel");
function makeRun(id: string, status: AgentRun["status"] = "completed"): AgentRun {
    return { id, conversationId: "shared-session", canvasId: "shared-canvas", status, userPrompt: `用户-${id}`, permissionMode: "request_approval", createdAt: "2026-10-23T01:00:00Z", updatedAt: `2026-10-23T01:00:0${id.length}Z`, revision: 1 };
}
function chat(id: string, latest: AgentRun | null): CloudAgentConversation {
    return {
        id,
        serverConversationId: latest?.conversationId,
        run: latest,
        lastEventSeq: 4,
        runs: latest ? [latest] : [],
        title: id,
        messages: latest
            ? [
                  { id: `user-${latest.id}`, role: "user", text: latest.userPrompt! },
                  { id: `assistant-${latest.id}`, role: "assistant", text: `结果-${latest.id}` },
              ]
            : [],
        permissionMode: "request_approval",
        createdAt: "2026-10-23T01:00:00Z",
        updatedAt: "2026-10-23T01:00:00Z",
    };
}
const settle = async () => {
    for (let i = 0; i < 5; i++) await new Promise((resolve) => setTimeout(resolve, 10));
};
const focus = async () => {
    await act(async () => {
        window.dispatchEvent(new Event("focus"));
        await settle();
    });
};
// 面板合并写入本机对话缓存；断言缓存前先触发与页面隐藏相同的立即写入（缓存 mock 同步落盘）。
const persist = async () => {
    await act(async () => {
        window.dispatchEvent(new Event("pagehide"));
    });
};
async function mount(initial: CloudAgentConversation, cloud: CloudAgentConversation[], initialPending: CloudAgentPendingSubmission | null = null, inCanvas = true) {
    local = [initial];
    activeId = initial.id;
    remote = cloud;
    pending = initialPending;
    refreshOverride = undefined;
    sent.length = 0;
    sendInputs.length = 0;
    createInputs.length = 0;
    sendFailure = undefined;
    capabilitiesOverride = undefined;
    sendOverride = undefined;
    createOverride = undefined;
    mediaPrepare = undefined;
    mediaDecision = undefined;
    generated = 0;
    subscriptions.length = 0;
    refreshRequests.length = 0;
    setActiveUserScope("sync-user");
    useUserStore.setState({ hydrated: true, user: { id: "sync-user", username: "test", displayName: "test", role: "user", status: "active" } });
    configureModels(false);
    useGlobalAgentStore.setState({ open: true, canvas: inCanvas ? { canvasId: "shared-canvas", nodeCount: 2, selectedNodeIds: [], references: [] } : null });
    const host = document.createElement("div");
    document.body.append(host);
    const root = createRoot(host);
    await act(async () => {
        root.render(<CanvasCloudAgentPanel />);
        await settle();
    });
    return {
        host,
        root,
        close: async () => {
            await act(async () => root.unmount());
            host.remove();
        },
    };
}
afterAll(() => dom.happyDOM.abort());
function configureModels(ambiguous: boolean) {
    const entry = (id: string, model: string, price: number) => createModelChannel({ id, scope: "system", models: [model], modelDetails: [{ model, channelModelId: `${id}-${model}`, capability: "text" }] });
    const channels = [entry("paid-channel", "paid-model", 10_000_000), entry("local-test", "local-agent-test", 0), ...(ambiguous ? [entry("another-test", "local-agent-test", 0)] : [])];
    const config = normalizeConfigSnapshot({ config: { ...defaultConfig, channels, textModel: "paid-channel::paid-model", model: "paid-channel::paid-model" } }).config;
    useConfigStore.setState({ config });
}

test.each(["已保存", "仍不可读", "切换画布", "切换账号"])("草稿画布首句：准备后%s", async (scenario) => {
    const calls: string[] = [];
    let prepared = false;
    draftProfile = async (id) => {
        calls.push(`profile:${id}`);
        if (!prepared || scenario === "仍不可读") throw new ApiError("画布不可读", { status: 404 });
    };
    prepareDraft = (id) => {
        calls.push(`prepare:${id}`);
        prepared = true;
        if (scenario === "切换画布") useGlobalAgentStore.setState({ canvas: null });
        if (scenario === "切换账号") setActiveUserScope("other");
    };
    const mounted = await mount(chat("draft", null), []);
    try {
        createOverride = async () => ({ run: makeRun("accepted", "running") });
        expect(calls).toEqual(["profile:shared-canvas"]);
        await act(async () => composer.onPromptChange("整理这张新画布"));
        await act(async () => {
            composer.onSubmit();
            composer.onSubmit();
            await settle();
        });
        expect(calls).toContain("prepare:shared-canvas");
        if (scenario === "已保存") {
            expect(calls).toEqual(["profile:shared-canvas", "prepare:shared-canvas", "profile:shared-canvas"]);
            expect(createInputs).toHaveLength(1);
            expect(createInputs[0]).toMatchObject({ canvasId: "shared-canvas", profileRevision: "profile", prompt: "整理这张新画布" });
        } else {
            expect(createInputs).toHaveLength(0);
            expect(pending).toBeNull();
            if (scenario === "仍不可读") expect(calls).toEqual(["profile:shared-canvas", "prepare:shared-canvas", "profile:shared-canvas"]);
        }
    } finally {
        draftProfile = undefined;
        prepareDraft = undefined;
        await mounted.close();
        setActiveUserScope("sync-user");
    }
});

test("画布偏好明确拒绝时不触发同步、保存请求或建轮", async () => {
    draftProfile = async () => {
        throw new ApiError("没有权限", { status: 403 });
    };
    let prepares = 0;
    prepareDraft = () => prepares++;
    const mounted = await mount(chat("forbidden", null), []);
    try {
        await act(async () => composer.onPromptChange("查看画布"));
        await act(async () => {
            composer.onSubmit();
            await settle();
        });
        expect(prepares).toBe(0);
        expect(createInputs).toHaveLength(0);
        expect(pending).toBeNull();
    } finally {
        draftProfile = undefined;
        prepareDraft = undefined;
        await mounted.close();
    }
});

test("草稿画布离线入队后恢复：预读 404 仍按原队列编号只建一轮", async () => {
    profileOffline = true;
    let prepared = false;
    draftProfile = async () => {
        if (!prepared) throw new ApiError("画布不可读", { status: 404 });
    };
    prepareDraft = () => {
        prepared = true;
    };
    const mounted = await mount(chat("offline-draft", null), []);
    try {
        createOverride = async () => ({ run: makeRun("accepted", "running") });
        await act(async () => composer.onPromptChange("待同步的草稿"));
        await act(async () => composer.onSubmit());
        const key = composer.tray.props.outbox.items[0].id;
        expect(createInputs).toHaveLength(0);
        expect(prepared).toBe(false);
        profileOffline = false;
        await act(async () => {
            window.dispatchEvent(new Event("online"));
            await settle();
        });
        expect(createInputs).toHaveLength(1);
        expect(createInputs[0]).toMatchObject({ canvasId: "shared-canvas", profileRevision: "profile", idempotencyKey: key });
        expect(mounted.host.querySelector(".agent-outbox-row")).toBeNull();
    } finally {
        profileOffline = false;
        draftProfile = undefined;
        prepareDraft = undefined;
        await mounted.close();
    }
});

test("无画布首句：浏览器误报离线但 Agent 可达时直接创建一轮", async () => {
    Object.defineProperty(navigator, "onLine", { configurable: true, value: false });
    const mounted = await mount(chat("original", null), [], null, false);
    try {
        createOverride = async () => ({ run: { ...makeRun("accepted", "running"), canvasId: "" } });
        expect(composer.submitMode).toBe("send");
        await act(async () => composer.onPromptChange("你好"));
        await act(async () => {
            composer.onSubmit();
            await settle();
        });
        expect(createInputs).toHaveLength(1);
        expect(createInputs[0]).toMatchObject({ prompt: "你好", canvasId: "" });
    } finally {
        await mounted.close();
        Reflect.deleteProperty(navigator, "onLine");
    }
});

test("无画布首句：真断网时保留队列，重新连接不依赖 online 事件且不重复建轮", async () => {
    const mounted = await mount(chat("original", null), [], null, false);
    Object.defineProperty(navigator, "onLine", { configurable: true, value: false });
    try {
        profileOffline = true;
        createOverride = async () => ({ run: { ...makeRun("accepted", "running"), canvasId: "" } });
        await act(async () => window.dispatchEvent(new Event("offline")));
        await act(async () => composer.onPromptChange("你好"));
        await act(async () => composer.onSubmit());
        const button = () => mounted.host.querySelector<HTMLButtonElement>(".agent-outbox-row .agent-outbox-action")!;
        expect(mounted.host.textContent).toContain("暂时连不上 Agent，消息已保留");
        expect(button().textContent).toBe("重新连接");
        expect(button().disabled).toBe(false);
        await act(async () => {
            button().click();
            await settle();
        });
        expect(createInputs).toHaveLength(0);
        expect(mounted.host.querySelector(".agent-outbox-text")?.textContent).toBe("你好");
        profileOffline = false;
        await act(async () => {
            button().click();
            button().click();
            await settle();
        });
        expect(createInputs).toHaveLength(1);
        expect(createInputs[0]).toMatchObject({ prompt: "你好", canvasId: "" });
        expect(mounted.host.querySelector(".agent-outbox-row")).toBeNull();
    } finally {
        profileOffline = false;
        await mounted.close();
        Reflect.deleteProperty(navigator, "onLine");
    }
});

test.each(["手动暂停", "自动继续"])("已入队首句：恢复连接后遵守%s且保持原消息身份", async (mode) => {
    profileOffline = true;
    const mounted = await mount(chat("original", null), [], null, false);
    try {
        createOverride = async () => ({ run: { ...makeRun("accepted", "running"), canvasId: "" } });
        // 系统报告在线也不能覆盖 Agent 接口没有收到响应的事实。
        expect(composer.submitMode).toBe("queue");
        await act(async () => composer.onPromptChange("等我联网"));
        await act(async () => composer.onSubmit());
        const key = composer.tray.props.outbox.items[0].id;
        if (mode === "手动暂停") await act(async () => composer.tray.props.onPause());
        profileOffline = false;
        if (mode === "手动暂停") await focus();
        else await act(async () => window.dispatchEvent(new Event("online")));
        if (mode === "手动暂停") {
            expect(createInputs).toHaveLength(0);
            expect(composer.tray.props.outbox.paused.reason).toBe("user");
            await act(async () => composer.tray.props.onResume());
        }
        expect(createInputs).toHaveLength(1);
        expect(createInputs[0]).toMatchObject({ prompt: "等我联网", idempotencyKey: key });
    } finally {
        profileOffline = false;
        await mounted.close();
    }
});

test("手机空缓存接续同画布桌面会话，审批被另一端处理后自动收敛且不重复提交", async () => {
    const waiting = makeRun("desktop", "waiting_approval");
    waiting.approval = { approvalId: "approval-one", call: { id: "call", function: { name: "generate_media", arguments: "{}" } } };
    const mounted = await mount(chat("new-phone", null), [chat("server", waiting)]);
    try {
        expect(conversationUI.messages.map((item: any) => item.text)).toContain("结果-desktop");
        expect(conversationUI.approval.approvalId).toBe("approval-one");
        expect(subscriptions.at(-1)).toMatchObject({ id: "desktop", after: 4 });
        await act(async () => {
            conversationUI.onApprovalReasonChange("手机保留的理由");
        });
        await focus();
        expect(conversationUI.approval.reason).toBe("手机保留的理由");
        const observation = subscriptions.at(-1)!;
        await act(async () => {
            observation.event({ eventId: "decision", runId: "desktop", seq: 5, type: "approval_decided", payload: { approvalId: "approval-one", decision: "approve" }, createdAt: waiting.updatedAt });
            observation.event({ eventId: "status", runId: "desktop", seq: 6, type: "run_status", payload: { status: "running", revision: 2 }, createdAt: waiting.updatedAt });
        });
        expect(conversationUI.approval).toBeNull();
        await persist();
        expect(local.find((item) => item.id === "server")?.lastEventSeq).toBe(6);
        expect(generated).toBe(0);
    } finally {
        await mounted.close();
    }
});

test("原生操作卡同意一次完成媒体准备与提交，拒绝跳过准备", async () => {
    for (const decision of ["approve", "reject", "auto"] as const) {
        const waiting = makeRun("media-run", "waiting_approval");
        waiting.approval = { approvalId: "media-draft", studioMedia: true, call: { id: "call", function: { name: "generate_media", arguments: "{}" } } };
        const mounted = await mount(chat("local", waiting), [chat("server", waiting)]);
        try {
            const calls: string[] = [];
            mediaPrepare = async () => {
                calls.push("prepare");
                return "frozen";
            };
            mediaDecision = (value, quoteId, enableAuto) => calls.push(`${value}:${quoteId || ""}${enableAuto ? ":auto" : ""}`);
            expect(conversationUI.approval.detail.studioMedia).toBe(true);
            expect(calls).toEqual([]);
            await act(async () => {
                if (decision === "approve") {
                    conversationUI.onApprove();
                    conversationUI.onApprove();
                } else if (decision === "auto") {
                    controls.onPermissionChange("auto");
                    controls.onPermissionChange("auto");
                } else conversationUI.onReject();
                await settle();
            });
            expect(calls).toEqual(decision === "reject" ? ["reject:"] : ["prepare", decision === "auto" ? "approve:frozen:auto" : "approve:frozen"]);
        } finally {
            await mounted.close();
        }
    }
});

test("旧轮次结束后仍发现手机新轮次，电脑下一条消息使用最新 child 续接", async () => {
    const first = chat("desktop-local", makeRun("first"));
    const mounted = await mount(first, [chat("server", makeRun("first"))]);
    try {
        expect(refreshRequests.some((item) => item.canvasId === "shared-canvas" && item.observedConversationIds?.includes("shared-session"))).toBe(true);
        remote = [chat("server", { ...makeRun("second"), parentId: "first" })];
        await focus();
        expect(conversationUI.messages.map((item: any) => item.text)).toContain("结果-second");
        expect(subscriptions.find((item) => item.id === "first")?.closed).toBe(true);
        expect(subscriptions.at(-1)?.id).toBe("second");
        await act(async () => {
            subscriptions.find((item) => item.id === "first")!.event({ eventId: "late-old-event", runId: "first", seq: 5, type: "assistant_delta", payload: { messageId: "old-output", text: "不应出现在新轮次" }, createdAt: first.updatedAt });
        });
        expect(conversationUI.messages.map((item: any) => item.text)).not.toContain("不应出现在新轮次");
        await act(async () => {
            composer.onPromptChange("电脑接着这一轮继续");
        });
        await act(async () => {
            composer.onSubmit();
            await settle();
        });
        expect(sent).toEqual(["second"]);
        expect(sendInputs[0]).not.toHaveProperty("budget");
        expect(generated).toBe(0);
        expect(pending).toBeNull();
    } finally {
        await mounted.close();
    }
});

test("失败后续发立即显示正在发送，迟到的上一轮错误留在新输入之前", async () => {
    const parent = { ...makeRun("first", "failed"), failureMessage: "上一轮模型暂时不可用" };
    const initial = chat("original", parent);
    initial.messages = initial.messages.slice(0, 1);
    const mounted = await mount(initial, [initial]);
    let accept!: (value: { run: AgentRun }) => void;
    try {
        sendOverride = () => new Promise((resolve) => (accept = resolve));
        await act(async () => composer.onPromptChange("再问一条"));
        await act(async () => {
            composer.onSubmit();
            await settle();
        });
        expect(sent).toEqual(["first"]);
        expect(header.statusLabel).toBe("正在发送");
        await act(async () => {
            subscriptions.at(-1)!.event({ eventId: "parent-failure", runId: parent.id, seq: 5, type: "run_failed", payload: { text: parent.failureMessage }, createdAt: parent.updatedAt });
        });
        expect(conversationUI.messages.map((message: any) => message.role)).toEqual(["user", "error", "user"]);
        expect(conversationUI.messages.at(-1).text).toBe("再问一条");
        await act(async () => {
            accept({ run: { ...makeRun("second", "running"), parentId: parent.id } });
            await settle();
        });
        expect(header.statusLabel).toBe("运行中");
        expect(subscriptions.at(-1)?.id).toBe("second");
        remote = [{ ...initial }];
        await focus();
        expect(header.statusLabel).toBe("运行中");
        expect(subscriptions.at(-1)?.id).toBe("second");
        expect(sent).toEqual(["first"]);
    } finally {
        await mounted.close();
    }
});

test.each(["首轮", "续轮"])("%s正常等待确认时显示正在发送，收到响应后进入运行状态", async (entry) => {
    const parent = entry === "续轮" ? makeRun("first") : null;
    const initial = chat("original", parent);
    const mounted = await mount(initial, [initial]);
    const result = { run: { ...makeRun("accepted", "running"), parentId: parent?.id } };
    let accept: ((value: { run: AgentRun }) => void) | undefined;
    try {
        sendOverride = createOverride = () => new Promise((resolve) => (accept = resolve));
        await act(async () => composer.onPromptChange("整理画布"));
        await act(async () => {
            composer.onSubmit();
            await settle();
        });
        expect(generated + sent.length).toBe(1);
        expect(pending?.request?.prompt).toBe("整理画布");
        expect(composer.prompt).toBe("");
        expect(header.statusLabel).toBe("正在发送");
        expect(mounted.host.querySelector(".agent-outbox-row.is-failed") === null).toBe(true);
        expect(conversationUI.messages.some((message: any) => message.role === "error")).toBe(false);
        await act(async () => {
            accept!(result);
            await settle();
        });
        expect(header.statusLabel).toBe("运行中");
        expect(pending).toBeNull();
        expect(mounted.host.querySelector(".agent-outbox-row.is-failed") === null).toBe(true);
        expect(subscriptions.at(-1)?.id).toBe("accepted");
        expect(generated + sent.length).toBe(1);
    } finally {
        await act(async () => {
            accept?.(result);
            await settle();
        });
        await mounted.close();
    }
});

test("网络失败后显示待确认操作，点击重试沿用原请求并收起失败条", async () => {
    const initial = chat("original", makeRun("first"));
    const mounted = await mount(initial, [initial]);
    const result = { run: { ...makeRun("accepted", "running"), parentId: "first" } };
    let accept: ((value: { run: AgentRun }) => void) | undefined;
    try {
        sendFailure = new Error("网络连接中断");
        await act(async () => composer.onPromptChange("原请求内容"));
        await act(async () => {
            composer.onSubmit();
            await settle();
        });
        const original = pending;
        expect(original?.request).toEqual(sendInputs[0]);
        expect(composer.prompt).toBe("原请求内容");
        expect(conversationUI.messages.some((message: any) => message.role === "error")).toBe(true);
        await act(async () => composer.onPromptChange("下一条草稿"));
        const retry = mounted.host.querySelector<HTMLButtonElement>(".agent-outbox-row.is-failed button");
        expect(retry).not.toBeNull();
        expect(retry!.disabled).toBe(false);
        sendFailure = undefined;
        sendOverride = () => new Promise((resolve) => (accept = resolve));
        await act(async () => {
            retry!.click();
            await settle();
        });
        expect(header.statusLabel).toBe("正在发送");
        expect(mounted.host.querySelector(".agent-outbox-row.is-failed") === null).toBe(true);
        expect(sent).toEqual(["first", "first"]);
        expect(sendInputs[1]).toEqual(original?.request);
        expect(composer.prompt).toBe("下一条草稿");
        await act(async () => {
            accept!(result);
            await settle();
        });
        expect(pending).toBeNull();
        expect(header.statusLabel).toBe("运行中");
        expect(composer.prompt).toBe("下一条草稿");
        expect(generated).toBe(0);
    } finally {
        await act(async () => {
            accept?.(result);
            await settle();
        });
        await mounted.close();
    }
});

test("刷新恢复的待确认首轮仍可重试，发送期间收起失败条并保留原幂等键", async () => {
    const original: CloudAgentPendingSubmission = {
        key: "restored-original-key",
        fingerprint: "frozen-body",
        request: { canvasId: "shared-canvas", prompt: "恢复原请求", idempotencyKey: "restored-original-key", budget: { maxSteps: 16 } },
    };
    const initial = chat("original", null);
    const mounted = await mount(initial, [initial], original);
    const result = { run: makeRun("restored", "running") };
    let accept: ((value: { run: AgentRun }) => void) | undefined;
    try {
        expect(composer.prompt).toBe("恢复原请求");
        expect(generated).toBe(0);
        await act(async () => composer.onPromptChange("下一条草稿"));
        const retry = mounted.host.querySelector<HTMLButtonElement>(".agent-outbox-row.is-failed button");
        expect(retry).not.toBeNull();
        createOverride = () => new Promise((resolve) => (accept = resolve));
        await act(async () => {
            retry!.click();
            await settle();
        });
        expect(header.statusLabel).toBe("正在发送");
        expect(mounted.host.querySelector(".agent-outbox-row.is-failed") === null).toBe(true);
        expect(createInputs).toEqual([original.request]);
        expect(pending).toEqual(original);
        await act(async () => {
            accept!(result);
            await settle();
        });
        expect(pending).toBeNull();
        expect(header.statusLabel).toBe("运行中");
        expect(composer.prompt).toBe("下一条草稿");
        expect(generated).toBe(1);
    } finally {
        await act(async () => {
            accept?.(result);
            await settle();
        });
        await mounted.close();
    }
});

test("云端发现不能替换未提交草稿，明确新建后不会重新接回旧会话", async () => {
    const initial = { ...chat("phone-draft", null), draft: "尚未发送的草稿" };
    const mounted = await mount(initial, [chat("server", makeRun("desktop"))]);
    try {
        expect(composer.prompt).toBe("尚未发送的草稿");
        expect(conversationUI.messages).toHaveLength(0);
        await act(async () => {
            header.onNew();
            await settle();
        });
        expect(composer.prompt).toBe("");
        await persist();
        expect(local.some((item) => item.id === "phone-draft" && item.draft === "尚未发送的草稿")).toBe(true);
        await focus();
        expect(conversationUI.messages).toHaveLength(0);
        await persist();
        expect(local.find((item) => item.id === activeId)?.localNewCanvasId).toBe("shared-canvas");
        expect(generated).toBe(0);
    } finally {
        await mounted.close();
    }
});

test("旧账号云端请求迟到时不得写入已切换账号的面板", async () => {
    const mounted = await mount(chat("safe-local", null), []);
    try {
        let release: ((value: CloudAgentConversation[]) => void) | undefined;
        refreshOverride = () =>
            new Promise((resolve) => {
                release = resolve;
            });
        await act(async () => {
            window.dispatchEvent(new Event("focus"));
            await settle();
        });
        expect(release).toBeDefined();
        await act(async () => {
            setActiveUserScope("other-user");
            useUserStore.setState({ user: { id: "other-user", username: "other", displayName: "other", role: "user", status: "active" } });
            release!([chat("old-account-secret", makeRun("secret"))]);
            await settle();
        });
        expect(conversationUI.messages.map((item: any) => item.text)).not.toContain("结果-secret");
        await persist();
        expect(local.some((item) => item.serverConversationId === "shared-session")).toBe(false);
    } finally {
        await mounted.close();
    }
});

test("手选同画布较早历史后不被其他会话抢走，但仍能跟随所选会话的新轮次", async () => {
    const earlier = chat("earlier", makeRun("earlier"));
    const newer = chat("newer", { ...makeRun("newer"), conversationId: "different-session" });
    const mounted = await mount(earlier, [earlier, newer]);
    try {
        await act(async () => {
            header.onHistory();
            await new Promise((resolve) => setTimeout(resolve, 350));
        });
        expect(historyUI).toBeDefined();
        await act(async () => {
            historyUI.onOpen(earlier);
            await new Promise((resolve) => setTimeout(resolve, 350));
        });
        await focus();
        expect(conversationUI.messages.map((item: any) => item.text)).toContain("结果-earlier");
        await persist();
        expect(local.find((item) => item.id === activeId)?.localSelectedCanvasId).toBe("shared-canvas");
        remote = [{ ...earlier, run: { ...makeRun("earlier-child"), parentId: "earlier" }, messages: [{ id: "new-result", role: "assistant", text: "所选会话的新轮次" }] }, newer];
        await focus();
        expect(conversationUI.messages.map((item: any) => item.text)).toContain("所选会话的新轮次");
    } finally {
        await mounted.close();
    }
});

test("未知提交的原请求与幂等身份保留，不能因发现云端同文本会话而被换掉", async () => {
    const original = { key: "original-key", fingerprint: "frozen-body", request: { canvasId: "shared-canvas", prompt: "生成森林", idempotencyKey: "original-key" } };
    const other = chat("other", { ...makeRun("other"), userPrompt: "生成森林", idempotencyKey: "another-key" });
    const mounted = await mount(chat("uncertain", null), [other], original);
    try {
        await focus();
        expect(pending).toEqual(original);
        expect(composer.prompt).toBe("生成森林");
        expect(conversationUI.messages).toHaveLength(0);
        await persist();
        expect(activeId).toBe("uncertain");
        expect(generated).toBe(0);
    } finally {
        await mounted.close();
    }
});

test("手机后台断线后在前台恢复同一事件订阅，focus与visibility不会发送或审批", async () => {
    const waiting = makeRun("background", "waiting_approval");
    waiting.approval = { approvalId: "remain-pending", call: { id: "call", function: { name: "generate_media", arguments: "{}" } } };
    const mounted = await mount(chat("mobile", waiting), [chat("server", waiting)]);
    try {
        const first = subscriptions.at(-1)!;
        await act(async () => {
            first.connection("disconnected");
        });
        const count = subscriptions.length;
        Object.defineProperty(document, "visibilityState", { configurable: true, get: () => "hidden" });
        await act(async () => {
            document.dispatchEvent(new Event("visibilitychange"));
            window.dispatchEvent(new Event("focus"));
            await settle();
        });
        expect(subscriptions).toHaveLength(count);
        Object.defineProperty(document, "visibilityState", { configurable: true, get: () => "visible" });
        await act(async () => {
            document.dispatchEvent(new Event("visibilitychange"));
            window.dispatchEvent(new Event("focus"));
            await settle();
        });
        expect(subscriptions).toHaveLength(count + 1);
        expect(first.closed).toBe(true);
        expect(subscriptions.at(-1)).toMatchObject({ id: "background", after: 4 });
        await act(async () => {
            subscriptions.at(-1)!.connection("disconnected");
        });
        await focus();
        expect(subscriptions).toHaveLength(count + 2);
        expect(conversationUI.approval.approvalId).toBe("remain-pending");
        expect(sent).toHaveLength(0);
        expect(generated).toBe(0);
    } finally {
        Object.defineProperty(document, "visibilityState", { configurable: true, get: () => "visible" });
        await mounted.close();
    }
});

test("第二端恢复裸模型key为唯一完整option，续发保持原零积分渠道", async () => {
    const cloud = { ...chat("server", { ...makeRun("mock-turn"), model: "local-agent-test" }), model: "local-agent-test" };
    const mounted = await mount(chat("new-device", null), [cloud]);
    try {
        expect(controls.selectedModel).toBe("local-test::local-agent-test");
        await act(async () => {
            composer.onPromptChange("接着免费测试模型继续");
        });
        await act(async () => {
            composer.onSubmit();
            await settle();
        });
        expect(sendInputs).toHaveLength(1);
        expect(sendInputs[0]).toMatchObject({ model: "local-agent-test", channelId: "local-test", channelModelKey: "local-agent-test" });
        expect(generated).toBe(0);
    } finally {
        await mounted.close();
    }
});

test("旧型号下架时显示原型号，确认使用默认文本模型仅更新选择", async () => {
    const mounted = await mount({ ...chat("retired", null), model: "old-channel::retired-model" }, []);
    try {
        expect(mounted.host.textContent).toContain("「retired-model」不在当前文本模型目录中");
        expect(composer.submitDisabled).toBe(true);
        const useDefault = Array.from(mounted.host.querySelectorAll("button")).find((button) => button.textContent?.includes("使用默认模型 · paid-model"));
        expect(useDefault).toBeDefined();
        await act(async () => useDefault!.click());
        expect(controls.selectedModel).toBe("paid-channel::paid-model");
        expect(composer.submitDisabled).toBe(false);
        await persist();
        expect(local.find((item) => item.id === activeId)?.model).toBe("paid-channel::paid-model");
        expect(sendInputs).toHaveLength(0);
    } finally {
        await mounted.close();
    }
});

test("同名多渠道不能自动换到默认收费模型，手动选择后才允许继续", async () => {
    const mounted = await mount(chat("new-device", null), []);
    try {
        await act(async () => {
            configureModels(true);
        });
        remote = [{ ...chat("server", { ...makeRun("ambiguous"), model: "local-agent-test" }), model: "local-agent-test" }];
        await focus();
        expect(controls.selectedModel).toBe("");
        expect(composer.submitDisabled).toBe(true);
        await persist();
        expect(local.find((item) => item.id === activeId)?.model).toBe("local-agent-test");
        await act(async () => {
            composer.onPromptChange("等待确认渠道的草稿");
            composer.onSubmit();
            await settle();
        });
        expect(sendInputs).toHaveLength(0);
        await act(async () => {
            controls.onModelChange("another-test::local-agent-test");
        });
        expect(controls.selectedModel).toBe("another-test::local-agent-test");
        expect(composer.submitDisabled).toBe(false);
        await act(async () => {
            composer.onSubmit();
            await settle();
        });
        expect(sendInputs[0]).toMatchObject({ channelId: "another-test", channelModelKey: "local-agent-test" });
    } finally {
        await mounted.close();
    }
});

test("另一端已推进parent的明确拒绝只同步上下文，保留草稿而不自动重发", async () => {
    const first = chat("local", makeRun("first"));
    const mounted = await mount(first, [first]);
    try {
        await act(async () => {
            composer.onPromptChange("确认新上下文后再发送这条");
        });
        remote = [chat("server", { ...makeRun("other-device-child"), parentId: "first" })];
        sendFailure = new ApiError("会话已在另一端推进", { status: 409, reason: "agent_conversation_stale_parent" });
        await act(async () => {
            composer.onSubmit();
            await settle();
        });
        expect(sent).toEqual(["first"]);
        expect(pending).toBeNull();
        expect(composer.prompt).toBe("确认新上下文后再发送这条");
        expect(conversationUI.messages.some((item: any) => item.text === "结果-other-device-child")).toBe(true);
        expect(conversationUI.messages.some((item: any) => item.role === "user" && item.text === "确认新上下文后再发送这条")).toBe(false);
        expect(generated).toBe(0);
    } finally {
        await mounted.close();
    }
});

test("同轮历史已追赶到新水位后，延迟的旧 SSE delta 不会再次追加正文", async () => {
    const current = chat("local", makeRun("streaming", "running"));
    const mounted = await mount(current, [current]);
    try {
        const observation = subscriptions.at(-1)!;
        remote = [{ ...current, run: { ...current.run!, revision: 2 }, lastEventSeq: 6, messages: [{ id: "live-answer", role: "assistant", text: "完整回答" }] }];
        await focus();
        expect(conversationUI.messages.find((item: any) => item.id === "live-answer")?.text).toBe("完整回答");
        expect(subscriptions.at(-1)).toBe(observation);
        await act(async () => {
            observation.event({ eventId: "delayed-five", runId: "streaming", seq: 5, type: "assistant_delta", payload: { messageId: "live-answer", text: "重复片段" }, createdAt: current.updatedAt });
        });
        expect(conversationUI.messages.find((item: any) => item.id === "live-answer")?.text).toBe("完整回答");
        await act(async () => {
            observation.event({ eventId: "new-seven", runId: "streaming", seq: 7, type: "assistant_delta", payload: { messageId: "live-answer", text: "新内容" }, createdAt: current.updatedAt });
        });
        expect(conversationUI.messages.find((item: any) => item.id === "live-answer")?.text).toBe("完整回答新内容");
        await persist();
        expect(local.find((item) => item.id === activeId)?.lastEventSeq).toBe(7);
        expect(generated).toBe(0);
    } finally {
        await mounted.close();
    }
});

test("等待能力校验时切换账号，旧账号的请求不得以新账号身份提交", async () => {
    const initial = chat("original", makeRun("first"));
    const mounted = await mount(initial, [initial]);
    let release: ((value: unknown) => void) | undefined;
    try {
        capabilitiesOverride = () =>
            new Promise((resolve) => {
                release = resolve;
            });
        await act(async () => {
            composer.onPromptChange("属于原账号的输入");
        });
        await act(async () => {
            composer.onSubmit();
            await settle();
        });
        expect(release).toBeDefined();
        await act(async () => {
            setActiveUserScope("other-user");
            useUserStore.setState({ user: { id: "other-user", username: "other", displayName: "other", role: "user", status: "active" } });
            release!({ permissionModes: ["request_approval"], skills: true });
            await settle();
        });
        expect(sent).toHaveLength(0);
        expect(pending).toBeNull();
        expect(composer.prompt).toBe("");
        expect(generated).toBe(0);
    } finally {
        release?.({ permissionModes: ["request_approval"], skills: true });
        await mounted.close();
    }
});

test("旧账号 POST 的迟到响应不覆盖新账号面板，清理也只使用原账号范围", async () => {
    const initial = chat("original", makeRun("first"));
    const mounted = await mount(initial, [initial]);
    let release: ((value: { run: AgentRun }) => void) | undefined;
    try {
        sendOverride = () =>
            new Promise((resolve) => {
                release = resolve;
            });
        await act(async () => {
            composer.onPromptChange("原账号已提交的输入");
        });
        await act(async () => {
            composer.onSubmit();
            await settle();
        });
        expect(sent).toEqual(["first"]);
        expect(release).toBeDefined();
        await act(async () => {
            setActiveUserScope("other-user");
            useUserStore.setState({ user: { id: "other-user", username: "other", displayName: "other", role: "user", status: "active" } });
            await settle();
            release!({ run: { ...makeRun("original-late-child"), parentId: "first" } });
            await settle();
        });
        expect(subscriptions.some((item) => item.id === "original-late-child")).toBe(false);
        expect(conversationUI.messages).toHaveLength(0);
        expect(composer.prompt).toBe("");
        expect(pending).toBeNull();
        expect(generated).toBe(0);
    } finally {
        release?.({ run: makeRun("original-late-child") });
        await mounted.close();
    }
});

test("原样重试未知 POST 后得到明确 stale_parent 可解除旧身份，仍保留草稿且不自动重发", async () => {
    const initial = chat("original", makeRun("first"));
    const original: CloudAgentPendingSubmission = {
        key: "uncertain-original-key",
        fingerprint: "frozen",
        parentRunId: "first",
        messageId: "user-uncertain-original-key",
        request: { canvasId: "shared-canvas", prompt: "原请求内容", idempotencyKey: "uncertain-original-key" },
    };
    const mounted = await mount(initial, [initial], original);
    try {
        remote = [chat("server", { ...makeRun("other-device-child"), parentId: "first" })];
        sendFailure = new ApiError("已确认该 key 未受理，另一端已推进", { status: 409, reason: "agent_conversation_stale_parent" });
        await act(async () => {
            composer.onSubmit();
            await settle();
        });
        expect(sent).toEqual(["first"]);
        expect(sendInputs[0]).toEqual(original.request);
        expect(pending).toBeNull();
        expect(composer.prompt).toBe("原请求内容");
        expect(conversationUI.messages.some((item: any) => item.text === "结果-other-device-child")).toBe(true);
        expect(generated).toBe(0);
    } finally {
        await mounted.close();
    }
});

test("手动重新打开当前云端历史时同步水位，旧 SSE 缓冲不会重复正文", async () => {
    const current = chat("local", makeRun("same-current"));
    const mounted = await mount(current, [current]);
    try {
        const observation = subscriptions.at(-1)!;
        await act(async () => {
            header.onHistory();
            await new Promise((resolve) => setTimeout(resolve, 350));
        });
        remote = [{ ...current, run: { ...current.run!, revision: 2 }, lastEventSeq: 6, messages: [{ id: "manual-history-answer", role: "assistant", text: "手动恢复的完整回复" }] }];
        await act(async () => {
            historyUI.onOpen(current);
            await new Promise((resolve) => setTimeout(resolve, 350));
        });
        expect(conversationUI.messages.find((item: any) => item.id === "manual-history-answer")?.text).toBe("手动恢复的完整回复");
        await act(async () => {
            observation.event({ eventId: "history-delayed-five", runId: "same-current", seq: 5, type: "assistant_delta", payload: { messageId: "manual-history-answer", text: "重复内容" }, createdAt: current.updatedAt });
        });
        expect(conversationUI.messages.find((item: any) => item.id === "manual-history-answer")?.text).toBe("手动恢复的完整回复");
        await persist();
        expect(local.find((item) => item.id === activeId)?.lastEventSeq).toBe(6);
        expect(generated).toBe(0);
    } finally {
        await mounted.close();
    }
});
