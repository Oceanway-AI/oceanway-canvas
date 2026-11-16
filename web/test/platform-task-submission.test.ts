import { describe, expect, test } from "bun:test";
import { submitPlatformTask } from "../src/services/platform-task-submission";
import type { CreateTaskInput, GenerationTask } from "../src/services/api/task-center";
import { ApiError } from "../src/services/api/request";

describe("Platform task submission", () => {
    test("a video request with an explicit ceiling keeps that ceiling when the create response is lost", async () => {
        const input: CreateTaskInput = { type: "canvas_video", prompt: "ocean", platform: { expectedPriceVersion: "p1", walletMaxPoints: "250" } };
        let posted: CreateTaskInput | undefined;
        const result = await submitPlatformTask(input, {
            scope: () => "alice",
            preview: async () => ({ previewUserId: "alice", previewBinding: "bound-quote", capability_revision: "a".repeat(64), connection_revision: 7, price_version: "p1", wallet_points: "198.72", allowed: true, combination_supported: true }),
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
                    preview: async () => ({ previewUserId: "alice", previewBinding: "bound-quote", capability_revision: "a".repeat(64), connection_revision: 7, price_version: "p1", wallet_points: "198.72", allowed: true, combination_supported: true }),
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
                    preview: async () => ({ previewUserId: "alice", previewBinding: "bound-quote", price_version: "p1", wallet_points: "1", allowed: true, combination_supported: true, ...revision }),
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
                return { previewUserId: "alice", previewBinding: "bound-quote", capability_revision: "a".repeat(64), connection_revision: 7, price_version: "p1", wallet_points: "0.123456789", allowed: true, combination_supported: true };
            },
            create: async (body) => {
                submissions.push(body);
                throw new Error("lost receipt");
            },
            recover: async (command, expectedUserId) => {
                recoveries++;
                expect(expectedUserId).toBe("alice");
                expect(command).toBe("frozen-media");
                return { id: "original" } as GenerationTask;
            },
        });
        expect(result.id).toBe("original");
        expect([previews, submissions.length, recoveries]).toEqual([1, 1, 1]);
        expect(submissions[0].platform).toEqual({ previewUserId: "alice", previewBinding: "bound-quote", expectedPriceVersion: "p1", expectedCapabilityRevision: "a".repeat(64), expectedConnectionRevision: 7 });
    });
    test("a stale quote never creates a paid task", async () => {
        let posted = 0;
        await expect(
            submitPlatformTask(
                { prompt: "cat", type: "canvas_image", platform: { expectedPriceVersion: "old" } },
                {
                    scope: () => "alice",
                    preview: async () => ({ previewUserId: "alice", previewBinding: "bound-quote", price_version: "new", wallet_points: "1", allowed: true, combination_supported: true }),
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

    const mediaInput = (): CreateTaskInput => ({ type: "canvas_image", prompt: "fixture", platform: { expectedPriceVersion: "p1" } });
    const offer = { previewUserId: "alice", previewBinding: "bound-quote", price_version: "p1", capability_revision: "a".repeat(64), connection_revision: 7, wallet_points: "1", allowed: true, combination_supported: true };

    test.each([{ previewUserId: "bob" }, { previewUserId: "" }, { previewBinding: "" }])("refuses another account or an unbound quote before creating a task: %j", async (changed) => {
        const calls: string[] = [];
        await expect(
            submitPlatformTask(mediaInput(), {
                scope: () => "alice",
                preview: async () => ({ ...offer, ...changed }),
                create: async () => {
                    calls.push("create");
                    return { id: "invalid" } as GenerationTask;
                },
                recover: async () => {
                    calls.push("recover");
                    return { id: "invalid" } as GenerationTask;
                },
            }),
        ).rejects.toThrow();
        expect(calls).toEqual([]);
    });

    test("an explicit server account change after preview never queries another account's command", async () => {
        const calls: string[] = [];
        let cookie = "alice";
        const rejected = new ApiError("账号已切换", { status: 409, reason: "studio_preview_account_changed" });
        await expect(
            submitPlatformTask(mediaInput(), {
                scope: () => "alice",
                preview: async () => {
                    cookie = "bob";
                    return offer;
                },
                create: async (body) => {
                    calls.push("create");
                    expect(body.platform?.previewUserId).toBe("alice");
                    expect(body.platform?.previewBinding).toBe("bound-quote");
                    expect(cookie).toBe("bob");
                    throw rejected;
                },
                recover: async () => {
                    calls.push("recover");
                    return { id: "other-task" } as GenerationTask;
                },
            }),
        ).rejects.toBe(rejected);
        expect(calls).toEqual(["create"]);
    });

    test("changing accounts cannot reuse the same prepared input", async () => {
        let userId = "alice";
        let creates = 0,
            previews = 0;
        const input = mediaInput();
        const deps = {
            scope: () => userId,
            preview: async () => {
                previews++;
                return offer;
            },
            create: async () => {
                creates++;
                return { id: "original" } as GenerationTask;
            },
            recover: async () => {
                throw new Error("unexpected");
            },
        };
        expect((await submitPlatformTask(input, deps)).id).toBe("original");
        userId = "bob";
        await expect(submitPlatformTask(input, deps)).rejects.toThrow("账号已切换");
        expect([creates, previews]).toEqual([1, 1]);
    });
});

test("published v1 media submits once across concurrent clicks and reopens the original command after an unknown response", async () => {
    const input: CreateTaskInput = { commandId: "same-v1-original", type: "canvas_image", prompt: "cat", platform: { expectedPriceVersion: "p", mediaGenerationV1: true } };
    let posts = 0,
        reads = 0,
        quotes = 0,
        unavailable = true;
    const deps = {
        scope: () => "v1-account",
        preview: async () => {
            quotes++;
            throw new Error("unused quote");
        },
        create: async () => {
            posts++;
            throw new Error("lost response");
        },
        recover: async (id: string, owner: string) => {
            reads++;
            expect([id, owner]).toEqual(["same-v1-original", "v1-account"]);
            if (unavailable) throw new Error("temporarily unavailable");
            return { id: "original" } as GenerationTask;
        },
    };
    await Promise.allSettled([submitPlatformTask(input, deps), submitPlatformTask({ ...input }, deps)]);
    unavailable = false;
    expect((await submitPlatformTask({ ...input }, deps)).id).toBe("original");
    expect([posts, reads, quotes]).toEqual([1, 2, 0]);
});
