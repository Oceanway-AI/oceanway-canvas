import { describe, expect, spyOn, test } from "bun:test";
import { submitPlatformTask } from "../src/services/platform-task-submission";
import { createGenerationTask, type CreateTaskInput, type GenerationTask } from "../src/services/api/task-center";
import { http } from "../src/services/api/request";

describe("Platform task submission", () => {
    test("a video request with an explicit ceiling keeps that ceiling when the create response is lost", async () => {
        const input: CreateTaskInput = { type: "canvas_video", prompt: "ocean", platform: { expectedPriceVersion: "p1", walletMaxPoints: "250" } };
        let posted: CreateTaskInput | undefined;
        const result = await submitPlatformTask(input, {
            scope: () => "alice",
            preview: async () => ({ capability_revision: "a".repeat(64), connection_revision: 7, price_version: "p1", wallet_points: "198.72", allowed: true, combination_supported: true }),
            create: async (body) => {
                posted = body;
                throw new Error("lost");
            },
            recover: async () => ({ id: "original" }) as GenerationTask,
        });
        expect(posted?.platform?.walletMaxPoints).toBe("250");
        expect(result.id).toBe("original");
    });
    test.each(["", "-1", "NaN", "198.7199999999999999999"])("video refuses invalid or insufficient explicit maximum %j before creating a task", async (maximum) => {
        let posted = 0;
        await expect(
            submitPlatformTask(
                { type: "canvas_video", prompt: "ocean", platform: { expectedPriceVersion: "p1", walletMaxPoints: maximum } },
                {
                    scope: () => "alice",
                    preview: async () => ({ capability_revision: "a".repeat(64), connection_revision: 7, price_version: "p1", wallet_points: "198.72", allowed: true, combination_supported: true }),
                    create: async () => {
                        posted++;
                        return { id: "bad" } as GenerationTask;
                    },
                    recover: async () => {
                        throw new Error("must not recover");
                    },
                },
            ),
        ).rejects.toThrow();
        expect(posted).toBe(0);
    });
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
            recoveries = 0;
        const submissions: CreateTaskInput[] = [];
        const input: CreateTaskInput = { commandId: "frozen-media", prompt: "cat", type: "canvas_image", platform: { expectedPriceVersion: "p1" } };
        const result = await submitPlatformTask(input, {
            scope: () => "alice",
            preview: async () => {
                previews++;
                return { capability_revision: "a".repeat(64), connection_revision: 7, price_version: "p1", wallet_points: "0.123456789", allowed: true, combination_supported: true };
            },
            create: async (body) => {
                submissions.push(body);
                throw new Error("lost receipt");
            },
            recover: async (command) => {
                recoveries++;
                expect(command).toBe("frozen-media");
                return { id: "original" } as GenerationTask;
            },
        });
        expect(result.id).toBe("original");
        expect([previews, submissions.length, recoveries]).toEqual([1, 1, 1]);
        expect(submissions[0].platform).toEqual({ expectedPriceVersion: "p1", expectedCapabilityRevision: "a".repeat(64), expectedConnectionRevision: 7 });
    });
    test.each(["canvas_image", "canvas_video"])("%s previews and submits directly without a spending ceiling", async (type) => {
        const input: CreateTaskInput = { prompt: "ocean", type, commandId: "direct-media", platform: { expectedPriceVersion: "p1" } };
        const quote = { capability_revision: "a".repeat(64), connection_revision: 7, price_version: "p1", wallet_points: "0.1234567890123456789", allowed: true, combination_supported: true };
        const post = spyOn(http, "post").mockResolvedValueOnce(quote).mockResolvedValueOnce({ id: "media", type, status: "queued" });
        try {
            const result = await createGenerationTask(input);
            expect(result.id).toBe("media");
            expect(post.mock.calls).toEqual([
                ["/tasks/preview", input],
                ["/tasks", { ...input, platform: { expectedPriceVersion: "p1", expectedCapabilityRevision: quote.capability_revision, expectedConnectionRevision: 7 } }],
            ]);
        } finally {
            post.mockRestore();
        }
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
