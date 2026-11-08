import { describe, expect, test } from "bun:test";
import { submitPlatformTask } from "../src/services/platform-task-submission";
import type { CreateTaskInput, GenerationTask } from "../src/services/api/task-center";

describe("Platform task submission", () => {
    test.each([
        {},
        { capability_revision: "a".repeat(64) },
        { capability_revision: "a".repeat(64), connection_revision: 0 },
        { capability_revision: "a".repeat(64), connection_revision: 1.5 },
        { capability_revision: "a".repeat(64), connection_revision: Number.MAX_SAFE_INTEGER + 1 },
        { capability_revision: "invalid", connection_revision: 7 },
    ])("incomplete revision preview never creates or recovers a task: %j", async (revision) => {
        let creates = 0,
            recoveries = 0;
        await expect(
            submitPlatformTask(
                { prompt: "cat", type: "canvas_image", platform: { expectedPriceVersion: "p1" } },
                {
                    scope: () => "alice",
                    preview: async () => ({ price_version: "p1", wallet_points: "1", allowed: true, combination_supported: true, ...revision }),
                    create: async () => {
                        creates++;
                        return { id: "invalid" } as GenerationTask;
                    },
                    recover: async () => {
                        recoveries++;
                        return { id: "invalid" } as GenerationTask;
                    },
                },
            ),
        ).rejects.toThrow("平台未批准");
        expect([creates, recoveries]).toEqual([0, 0]);
    });
    test("lost media create response retains original preview revisions and only queries its command", async () => {
        let previews = 0,
            creates = 0,
            recoveries = 0;
        const input: CreateTaskInput = { commandId: "frozen-media", prompt: "cat", type: "canvas_image", platform: { expectedPriceVersion: "p1" } };
        const result = await submitPlatformTask(input, {
            scope: () => "alice",
            preview: async () => {
                previews++;
                return { capability_revision: "a".repeat(64), connection_revision: 7, price_version: "p1", wallet_points: "0.123456789", allowed: true, combination_supported: true };
            },
            create: async (body) => {
                creates++;
                expect(body.platform).toEqual({ expectedPriceVersion: "p1", expectedCapabilityRevision: "a".repeat(64), expectedConnectionRevision: 7, walletMaxPoints: "0.123456789" });
                throw new Error("lost receipt");
            },
            recover: async (command) => {
                recoveries++;
                expect(command).toBe("frozen-media");
                return { id: "original" } as GenerationTask;
            },
        });
        expect(result.id).toBe("original");
        expect([previews, creates, recoveries]).toEqual([1, 1, 1]);
    });
    test("media freezes the exact decimal quote as its ceiling before submit", async () => {
        let posted = 0;
        const result = await submitPlatformTask(
            { prompt: "cat", type: "canvas_image", platform: { expectedPriceVersion: "p1" } },
            {
                scope: () => "alice",
                preview: async () => ({ capability_revision: "a".repeat(64), connection_revision: 7, price_version: "p1", wallet_points: "0.1234567890123456789", allowed: true, combination_supported: true }),
                create: async (input) => {
                    posted++;
                    expect(input.platform?.walletMaxPoints).toBe("0.1234567890123456789");
                    expect(input.platform).toMatchObject({ expectedCapabilityRevision: "a".repeat(64), expectedConnectionRevision: 7 });
                    return { id: "media" } as GenerationTask;
                },
                recover: async () => {
                    throw new Error("not needed");
                },
            },
        );
        expect(posted).toBe(1);
        expect(result.id).toBe("media");
    });
    test("a stale quote never creates a paid task", async () => {
        let posted = 0;
        await expect(
            submitPlatformTask(
                { prompt: "cat", type: "canvas_image", platform: { expectedPriceVersion: "old" } },
                {
                    scope: () => "alice",
                    preview: async () => ({ price_version: "new", wallet_points: "1", allowed: true, combination_supported: true }),
                    create: async () => {
                        posted++;
                        return { id: "media" } as GenerationTask;
                    },
                    recover: async () => {
                        throw new Error("not needed");
                    },
                },
            ),
        ).rejects.toThrow("平台未批准");
        expect(posted).toBe(0);
    });
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
