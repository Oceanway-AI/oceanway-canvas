import { createClientUUID } from "@/lib/client-id";
import type { CreateTaskInput, GenerationTask } from "@/services/api/task-center";

type SubmissionDependencies = {
    create(input: CreateTaskInput): Promise<GenerationTask>;
    recover(commandId: string): Promise<GenerationTask>;
    scope(): string;
};

const commands = new WeakMap<CreateTaskInput, string>();

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
    let task: GenerationTask;
    try {
        task = await deps.create({ ...input, commandId });
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
