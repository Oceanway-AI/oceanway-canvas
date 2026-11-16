import { describe, expect, test } from "bun:test";

import { creationMessageNeedsResultSync, reconcileCreationTaskMessages, studioCreationOutputSnapshot, type PersistedCreationTask } from "@/pages/create/creation-conversations";
import type { CreationConversation, CreationMessage } from "@/pages/create/creation-types";
import { pendingCreationTaskIds } from "@/services/creation-conversation-store";
import { recoverCreationTextTask } from "@/services/creation-text-task-recovery";

const runtime = { recoverCreationTextTask } as unknown as Parameters<typeof reconcileCreationTaskMessages>[0];
const at = "2026-10-27T11:00:00.000Z";

function task(overrides: Partial<PersistedCreationTask> = {}): PersistedCreationTask {
    return { id: "task-1", type: "canvas_video", status: "running", prompt: "镜头", attempts: 1, createdAt: at, updatedAt: "2026-10-27T11:02:00.000Z", ...overrides };
}

function assistant(overrides: Partial<CreationMessage> = {}): CreationMessage {
    return { id: "assistant-1", role: "assistant", mode: "video", content: "", createdAt: at, taskIds: ["task-1"], ...overrides };
}

function conversations(message: CreationMessage): CreationConversation[] {
    return [{ id: "conversation-1", title: "镜头", updatedAt: at, messages: [{ id: "user-1", role: "user", mode: "video", content: "镜头", createdAt: at }, message] }];
}

const messageOf = (value: CreationConversation[]) => value[0].messages[1];

test("reopening a pending creation observes only the original task preview slots", () => {
    const observed = task({ provider: "platform", earlyPreviews: [{ index: 2, kind: "video" }], clientContext: { conversationId: "conversation-1", messageId: "assistant-1" } });
    const before = conversations(assistant({ status: "pending", taskIds: [] }));
    const pending = reconcileCreationTaskMessages(runtime, before, [observed]);
    expect(messageOf(pending)).toMatchObject({ status: "pending", taskIds: ["task-1"], earlyPreviews: [{ taskId: "task-1", index: 2, kind: "video" }] });
    expect(messageOf(pending).resultUrls).toBeUndefined();
    expect(messageOf(pending).resultStorageKeys).toBeUndefined();
    expect(reconcileCreationTaskMessages(runtime, pending, [observed])).toBe(pending);
});

describe("首页创作：页面没跟上 ≠ 生成失败", () => {
    test("轮询中断后任务仍在跑：失败消息回到生成中，并带上真实阶段与进度", () => {
        const next = messageOf(reconcileCreationTaskMessages(runtime, conversations(assistant({ status: "error", errorKind: "sync", error: "网络异常。", content: "生成失败" })), [task({ stage: "上游生成中", progress: 40 })]));
        expect(next).toMatchObject({ status: "pending", generationStatus: "running", generationStage: "上游生成中", generationProgress: 40 });
        expect(next.error).toBeUndefined();
        expect(next.errorKind).toBeUndefined();
        expect(next.content).toBe("");
    });

    test("旧版本未分类的“网络异常”失败，在任务成功后补回作品", () => {
        const legacy = assistant({ status: "error", error: "网络异常。", content: "生成失败" });
        expect(creationMessageNeedsResultSync(legacy)).toBe(true);
        const next = messageOf(reconcileCreationTaskMessages(runtime, conversations(legacy), [task({ status: "succeeded", creationResultStorageKeys: ["resource:video-1"] })]));
        expect(next).toMatchObject({ status: "done", content: "视频已生成", resultStorageKeys: ["resource:video-1"] });
        expect(next.error).toBeUndefined();
    });

    test("任务成功但结果暂时读不出来：保持“结果待同步”，不引导重新生成", () => {
        const next = messageOf(reconcileCreationTaskMessages(runtime, conversations(assistant({ status: "pending", syncInterrupted: true })), [task({ status: "succeeded", creationError: "Failed to fetch" })]));
        expect(next).toMatchObject({ status: "error", errorKind: "sync", error: "网络异常。" });
        expect(next.syncInterrupted).toBeUndefined();
        expect(creationMessageNeedsResultSync(next)).toBe(true);
    });

    test("后端确认失败：标记为任务失败；作品已生成可重新保存时保留入口", () => {
        const next = messageOf(reconcileCreationTaskMessages(runtime, conversations(assistant({ status: "pending" })), [task({ status: "failed", error: "作品已生成，但保存未完成：上传 OSS", mediaStage: "upload", canRecoverMedia: true })]));
        expect(next).toMatchObject({ status: "error", errorKind: "task", mediaRecoverable: true });
        expect(creationMessageNeedsResultSync(next)).toBe(false);
    });

    test("后端已确认失败的消息不会被之后的快照改写", () => {
        const source = conversations(assistant({ status: "error", errorKind: "task", error: "模型服务处理失败" }));
        expect(reconcileCreationTaskMessages(runtime, source, [task({ status: "succeeded", creationResultStorageKeys: ["resource:video-1"] })])).toBe(source);
    });

    test("阶段和进度没有变化时不产生新对象，避免每次轮询都重写本地记录", () => {
        const source = conversations(assistant({ status: "pending", generationStatus: "running", generationStage: "上游生成中", generationProgress: 40 }));
        expect(reconcileCreationTaskMessages(runtime, source, [task({ stage: "上游生成中", progress: 40 })])).toBe(source);
    });

    test("多图任务要等全部任务快照到齐才下结论", () => {
        const source = conversations(assistant({ mode: "image", status: "pending", taskIds: ["task-a", "task-b"] }));
        const first = task({ id: "task-a", type: "canvas_image", status: "succeeded", creationResultStorageKeys: ["resource:a"], clientContext: { conversationId: "conversation-1", messageId: "assistant-1", batchIndex: 0, batchCount: 2 } });
        expect(reconcileCreationTaskMessages(runtime, source, [first])).toBe(source);
        const second = task({ id: "task-b", type: "canvas_image", status: "failed", error: "上游拒绝", clientContext: { conversationId: "conversation-1", messageId: "assistant-1", batchIndex: 1, batchCount: 2 } });
        expect(messageOf(reconcileCreationTaskMessages(runtime, source, [first, second]))).toMatchObject({ status: "done", resultStorageKeys: ["resource:a"], content: "1 张图片已生成，1 张失败" });
    });

    test("手动重新获取可以重算已完成但没有可预览结果的消息", () => {
        const source = conversations(assistant({ status: "done", content: "视频已生成" }));
        const snapshot = [task({ status: "succeeded", creationResultStorageKeys: ["resource:video-1"] })];
        expect(reconcileCreationTaskMessages(runtime, source, snapshot)).toBe(source);
        expect(messageOf(reconcileCreationTaskMessages(runtime, source, snapshot, { reviveMessageIds: new Set(["assistant-1"]) }))).toMatchObject({ status: "done", resultStorageKeys: ["resource:video-1"] });
    });

    test("文本消息恢复监听拿到进行中状态后收起连接中断提示", () => {
        const source = conversations(assistant({ mode: "text", status: "pending", syncInterrupted: true, content: "第一段" }));
        const next = messageOf(reconcileCreationTaskMessages(runtime, source, [task({ type: "canvas_text" })]));
        expect(next.syncInterrupted).toBeUndefined();
        expect(next).toMatchObject({ status: "pending", content: "第一段" });
    });
});

describe("首页创作恢复监听范围", () => {
    test("生成中、结果待同步与旧版本失败继续跟进；后端确认失败的不再订阅", () => {
        const stored = [
            {
                id: "conversation-1",
                messages: [
                    { id: "pending", role: "assistant" as const, mode: "video", status: "pending", taskIds: ["task-pending"] },
                    { id: "sync", role: "assistant" as const, mode: "video", status: "error", errorKind: "sync", taskIds: ["task-sync"] },
                    { id: "legacy", role: "assistant" as const, mode: "video", status: "error", taskIds: ["task-legacy"] },
                    { id: "failed", role: "assistant" as const, mode: "video", status: "error", errorKind: "task", taskIds: ["task-failed"] },
                    { id: "done", role: "assistant" as const, mode: "video", status: "done", taskIds: ["task-done"] },
                ],
            },
        ];
        expect(pendingCreationTaskIds(stored)).toEqual(["task-pending", "task-sync", "task-legacy"]);
    });
});

test("create workbench shows already adopted original slots while the same command has another slot saving", () => {
    const outputs = [
        { index: 0, kind: "image", materialState: "saved", resourceId: "r0", media: { storageKey: "resource:r0", dataUrl: "/api/resources/r0/file" } },
        { index: 1, kind: "image", materialState: "saving" },
        { index: 2, kind: "image", materialState: "saved", resourceId: "r2", media: { storageKey: "resource:r2", dataUrl: "/api/resources/r2/file" } },
    ];
    const observed = task({
        type: "canvas_image",
        provider: "platform",
        platformGenerationRequest: "r",
        platformRevision: 2,
        resultJson: JSON.stringify({ outputs }),
        earlyPreviews: [
            { index: 0, kind: "image" },
            { index: 1, kind: "image" },
        ],
    });
    const next = reconcileCreationTaskMessages(runtime, conversations(assistant({ mode: "image", status: "pending" })), [observed]);
    expect(next[0].messages[1]).toMatchObject({ status: "pending", resultStorageKeys: ["resource:r0", "resource:r2"], earlyPreviews: [{ taskId: observed.id, index: 1, kind: "image" }] });
});

test("the first saved video receipt retains its authorized original preview while playback is being prepared", () => {
    const first = task({
        provider: "platform",
        status: "succeeded",
        platformRevision: 3,
        platformGenerationRequest: "already-saved",
        earlyPreviews: [{ index: 0, kind: "video" }],
        resultJson: JSON.stringify({ outputs: [{ index: 0, kind: "video", materialState: "saved", resourceId: "r-video", media: { storageKey: "resource:r-video", dataUrl: "/api/resources/r-video/file" } }] }),
    });
    expect(studioCreationOutputSnapshot([first])).toMatchObject({ earlyPreviews: [{ taskId: first.id, index: 0, kind: "video" }], resultStorageKeys: ["resource:r-video"] });
});
