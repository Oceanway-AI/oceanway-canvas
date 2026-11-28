import { sha256 } from "@noble/hashes/sha2.js";
import { bytesToHex } from "@noble/hashes/utils.js";
import { createClientUUID } from "@/lib/client-id";
import { ApiError } from "@/services/api/request";
import type { CreateTaskInput, GenerationTask } from "@/services/api/task-center";
import { validPlatformBudget, validPlatformPoints } from "./platform-budget-confirmation";

type SubmissionDependencies = {
    preview?(input: CreateTaskInput): Promise<{ previewUserId: string; previewBinding: string; capability_revision?: string; connection_revision?: number; price_version: string; wallet_points: string; allowed: boolean; combination_supported: boolean }>;
    create(input: CreateTaskInput): Promise<GenerationTask>;
    recover(commandId: string, expectedUserId: string): Promise<GenerationTask>;
    scope(): string;
};

const submissions = new WeakMap<CreateTaskInput, { owner: string; commandId: string }>();

// StudioCreateTask and StudioTaskByCommand both reject other IDs before looking up or creating a task.
export class InvalidStudioGenerationCommand extends Error {
    constructor() {
        super("这次提交的任务编号无效，未创建生成任务。请重新编辑后提交。");
    }
}

export function assertStudioGenerationCommand(commandId: string) {
    if (!commandId || commandId.length > 128 || /[^A-Za-z0-9_-]/.test(commandId)) throw new InvalidStudioGenerationCommand();
}

// A lost create response only reads the original command. No second POST is
// made, and an account switch cannot adopt another account's pending task.
export async function submitPlatformTask(input: CreateTaskInput, deps: SubmissionDependencies) {
    const scope = deps.scope();
    const assertScope = () => {
        if (!scope || deps.scope() !== scope) throw new Error("账号已切换，请在原账号中查看任务状态");
    };
    assertScope();
    let submission = submissions.get(input);
    if (!submission) {
        const metadata = input.input?.metadata as Record<string, unknown> | undefined;
        const commandId = input.commandId || (typeof metadata?.clientOperationId === "string" ? metadata.clientOperationId : createClientUUID());
        submission = { owner: scope, commandId };
        submissions.set(input, submission);
    }
    if (submission.owner !== scope) throw new Error("账号已切换，请在原账号中查看任务状态");
    const { commandId } = submission;
    assertStudioGenerationCommand(commandId);
    if (input.platform && (input.platform.mediaGenerationV1 || input.type === "canvas_image" || input.type === "canvas_video")) return submitMediaCommand(input, scope, commandId, deps, assertScope);
    let task: GenerationTask;
    try {
        task = await deps.create({ ...input, commandId });
    } catch (error) {
        assertScope();
        if (error instanceof ApiError && error.reason === "studio_preview_account_changed") throw error;
        try {
            task = await deps.recover(commandId, scope);
        } catch (recoveryError) {
            if (recoveryError instanceof ApiError && recoveryError.reason === "studio_preview_account_changed") throw recoveryError;
            assertScope();
            throw error;
        }
    }
    assertScope();
    return task;
}

async function prepareQuotedCommand(input: CreateTaskInput, scope: string, deps: SubmissionDependencies, assertScope: () => void): Promise<CreateTaskInput> {
    if (!deps.preview || !input.platform) throw new Error("平台报价暂不可用");
    const quote = await deps.preview(input);
    assertScope();
    const platform = input.platform;
    if (
        !quote.allowed ||
        !quote.combination_supported ||
        quote.price_version !== platform.expectedPriceVersion ||
        !validPlatformPoints(quote.wallet_points) ||
        !/^[a-f0-9]{64}$/.test(quote.capability_revision || "") ||
        !Number.isSafeInteger(quote.connection_revision) ||
        (quote.connection_revision ?? 0) <= 0 ||
        (platform.expectedCapabilityRevision && platform.expectedCapabilityRevision !== quote.capability_revision) ||
        (platform.expectedConnectionRevision !== undefined && platform.expectedConnectionRevision !== quote.connection_revision)
    )
        throw new Error("平台未批准当前参数或额度，请刷新报价");
    if (!quote.previewBinding || quote.previewUserId !== scope) throw new Error("账号已切换或报价已失效，请重新获取报价");
    if (platform.walletMaxPoints !== undefined && !validPlatformBudget(platform.walletMaxPoints, quote.wallet_points)) throw new Error("最高消费额度无效或低于预估");
    return { ...input, platform: { ...platform, previewUserId: quote.previewUserId, previewBinding: quote.previewBinding, expectedCapabilityRevision: quote.capability_revision, expectedConnectionRevision: quote.connection_revision } };
}

const activeCommands = new Map<string, { intent: string; promise: Promise<GenerationTask> }>();
const sentCommands = new Map<string, string>();

// Only a digest and the original command identifier survive page closure.
function stableIntent(input: CreateTaskInput) {
    return JSON.stringify(input, (_key, value) =>
        value && typeof value === "object" && !Array.isArray(value)
            ? Object.fromEntries(
                  Object.keys(value)
                      .sort()
                      .map((key) => [key, value[key]]),
              )
            : value,
    );
}

function commandKey(owner: string, workspace: string, commandId: string) {
    return JSON.stringify(["studio-media-v1", owner, workspace, commandId]);
}

export function studioGenerationCommandWasSent(owner: string, commandId: string, workspace = "") {
    const key = commandKey(owner, workspace, commandId);
    return sentCommands.has(key) || (typeof window !== "undefined" && window.localStorage.getItem(key) !== null);
}

async function submitMediaCommand(input: CreateTaskInput, owner: string, commandId: string, deps: SubmissionDependencies, assertScope: () => void) {
    const key = commandKey(owner, input.platform?.workspaceId || "", commandId);
    const intent = stableIntent({ ...input, commandId });
    const running = activeCommands.get(key);
    if (running) {
        if (running.intent !== intent) throw new Error("此任务命令已用于不同的创作意图");
        return running.promise;
    }
    const run = async () => {
        const digest = bytesToHex(sha256(new TextEncoder().encode(intent)));
        assertScope();
        const sent = sentCommands.get(key) ?? (typeof window === "undefined" ? null : window.localStorage.getItem(key));
        if (sent && sent !== digest) throw new Error("此任务命令已用于不同的创作意图");
        if (!sent) {
            const prepared = input.platform!.mediaGenerationV1 ? { ...input, commandId, platform: { ...input.platform!, expectedUserId: owner } } : await prepareQuotedCommand({ ...input, commandId }, owner, deps, assertScope);
            assertScope();
            if (typeof window !== "undefined") window.localStorage.setItem(key, digest);
            sentCommands.set(key, digest);
            try {
                const task = await deps.create(prepared);
                assertScope();
                return task;
            } catch (error) {
                assertScope();
                if (error instanceof ApiError && error.status && error.status >= 400 && error.status < 500) {
                    if (["studio_preview_account_changed", "studio_preview_selection_changed"].includes(error.reason || "")) {
                        sentCommands.delete(key);
                        if (typeof window !== "undefined") window.localStorage.removeItem(key);
                    }
                    throw error;
                }
            }
        }
        const task = await deps.recover(commandId, owner);
        assertScope();
        return task;
    };
    const promise = run().finally(() => {
        if (activeCommands.get(key)?.promise === promise) activeCommands.delete(key);
    });
    activeCommands.set(key, { intent, promise });
    return promise;
}
