import { createClientUUID } from "@/lib/client-id";
import type { CreateTaskInput, GenerationTask } from "@/services/api/task-center";

type SubmissionDependencies = {
    preview?(input: CreateTaskInput): Promise<{ capability_revision?: string; connection_revision?: number; price_version: string; wallet_points: string; allowed: boolean; combination_supported: boolean }>;
    create(input: CreateTaskInput): Promise<GenerationTask>;
    recover(commandId: string): Promise<GenerationTask>;
    scope(): string;
};

const commands = new WeakMap<CreateTaskInput, string>();
const preparedRequests = new WeakMap<CreateTaskInput, CreateTaskInput>();

// A lost create response only reads the original command. No second POST is
// made, and an account switch cannot adopt another account's pending task.
export async function submitPlatformTask(input: CreateTaskInput, deps: SubmissionDependencies) {
    const metadata = input.input?.metadata as Record<string, unknown> | undefined;
    const commandId = input.commandId || commands.get(input) || (typeof metadata?.clientOperationId === "string" ? metadata.clientOperationId : createClientUUID());
    commands.set(input, commandId);
    const scope = deps.scope();
    const assertScope = () => {
        if (!scope || deps.scope() !== scope) throw new Error("账号已切换，请在原账号中查看任务状态");
    };
    assertScope();
    let prepared = preparedRequests.get(input);
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
                !/^(0|[1-9]\d*)(\.\d+)?$/.test(quote.wallet_points) ||
                !/^[a-f0-9]{64}$/.test(quote.capability_revision || "") ||
                !Number.isSafeInteger(quote.connection_revision) ||
                (quote.connection_revision ?? 0) <= 0
            )
                throw new Error("平台未批准当前参数或额度，请刷新报价");
            prepared = { ...prepared, platform: { ...input.platform, expectedCapabilityRevision: quote.capability_revision, expectedConnectionRevision: quote.connection_revision, walletMaxPoints: input.platform.walletMaxPoints ?? quote.wallet_points } };
        }
        preparedRequests.set(input, prepared);
    }
    let task: GenerationTask;
    try {
        task = await deps.create(prepared);
    } catch (error) {
        assertScope();
        try {
            task = await deps.recover(commandId);
        } catch {
            assertScope();
            throw error;
        }
    }
    assertScope();
    return task;
}
