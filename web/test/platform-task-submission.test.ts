import { describe, expect, test } from "bun:test";
import { submitPlatformTask } from "../src/services/platform-task-submission";
import type { GenerationTask } from "../src/services/api/task-center";

describe("Platform task submission", () => {
    test("lost create response reads the same command without another paid request", async () => {
        let submitted = 0;
        let command = "";
        const task = { id: "original" } as GenerationTask;
        const result = await submitPlatformTask(
            { prompt: "hello", input: { metadata: { clientOperationId: "intent-1" } } },
            {
                scope: () => "alice",
                create: async (input) => {
                    submitted++;
                    command = input.commandId!;
                    throw new Error("connection lost");
                },
                recover: async (id) => {
                    expect(id).toBe(command);
                    return task;
                },
            },
        );
        expect(command).toBe("intent-1");
        expect(submitted).toBe(1);
        expect(result).toBe(task);
    });
    test("account switch never recovers or adopts a prior user's task", async () => {
        let scope = "alice";
        let reads = 0;
        await expect(
            submitPlatformTask(
                { prompt: "hello" },
                {
                    scope: () => scope,
                    create: async () => {
                        scope = "bob";
                        throw new Error("lost");
                    },
                    recover: async () => {
                        reads++;
                        return { id: "other" } as GenerationTask;
                    },
                },
            ),
        ).rejects.toThrow("账号已切换");
        expect(reads).toBe(0);
    });
});
