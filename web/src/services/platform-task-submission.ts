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

const submissions = new WeakMap<CreateTaskInput, { owner: string; commandId: string; prepared?: CreateTaskInput }>();

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
    let prepared = submission.prepared;
    if (!prepared) {
        prepared = { ...input, commandId };
        if (input.platform && (input.type === "canvas_image" || input.type === "canvas_video")) {
            if (!deps.preview) throw new Error("平台报价暂不可用");
            const quote = await deps.preview(prepared);
            assertScope();
            if (
                !quote.allowed ||
                !quote.combination_supported ||
                quote.price_version !== input.platform.expectedPriceVersion ||
                !validPlatformPoints(quote.wallet_points) ||
                !/^[a-f0-9]{64}$/.test(quote.capability_revision || "") ||
                !Number.isSafeInteger(quote.connection_revision) ||
                (quote.connection_revision ?? 0) <= 0
            )
                throw new Error("平台未批准当前参数或额度，请刷新报价");
            if (typeof quote.previewBinding !== "string" || !quote.previewBinding || quote.previewUserId !== scope) throw new Error("账号已切换或报价已失效，请重新获取报价");
            const maximum = input.platform.walletMaxPoints;
            if (maximum !== undefined && !validPlatformBudget(maximum, quote.wallet_points)) throw new Error("最高消费额度无效或低于预估");
            prepared = {
                ...prepared,
                platform: {
                    ...input.platform,
                    previewUserId: quote.previewUserId,
                    previewBinding: quote.previewBinding,
                    expectedCapabilityRevision: quote.capability_revision,
                    expectedConnectionRevision: quote.connection_revision,
                },
            };
        }
        submission.prepared = prepared;
    }
    let task: GenerationTask;
    try {
        task = await deps.create(prepared);
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
