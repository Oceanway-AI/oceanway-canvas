const assert = require("node:assert/strict");
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || "playwright");
const origin = new URL(process.env.STUDIO_MEDIA_BROWSER_ORIGIN);
assert.equal(origin.hostname, "127.0.0.1");
const endpoint = (path) => new URL(path, origin);
const counts = async () => (await fetch(endpoint("/api/__fixture/counts"))).json();
const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

async function until(check) {
    for (let attempt = 0; attempt < 150; attempt++) {
        if (await check()) return;
        await sleep(150);
    }
    throw Error("The expected browser observation did not arrive");
}

async function restrictedContext(browser, options) {
    const context = await browser.newContext(options);
    await context.route("**/*", (route) => {
        const url = new URL(route.request().url());
        const allowed = url.origin === origin.origin || (url.hostname === "127.0.0.1" && url.pathname.startsWith("/api/platform/v1/media-previews/"));
        return /^https?:/.test(url.protocol) && !allowed ? route.abort() : route.continue();
    });
    return context;
}

async function scenario(browser, loseResponse) {
    const before = await counts();
    const context = await restrictedContext(browser);
    try {
        const page = await context.newPage();
        await page.goto(endpoint("/create").href);
        await page.getByRole("button", { name: "选择图片模型" }).getByText("Fixture Image").waitFor();
        await page.locator(".creation-chat-mention-editor:visible").first().fill("Free local original media frame");
        let receive, release;
        const accepted = new Promise((resolve) => {
            receive = resolve;
        });
        const blocked = new Promise((resolve) => {
            release = resolve;
        });
        if (loseResponse) {
            await page.route("**/api/tasks", async (route) => {
                if (route.request().method() !== "POST") return route.continue();
                const response = await route.fetch();
                assert.equal(response.status(), 200);
                receive((await response.json()).data);
                await blocked;
            });
        }
        const ack = loseResponse
            ? accepted
            : page
                  .waitForResponse((response) => response.request().method() === "POST" && new URL(response.url()).pathname === "/api/tasks")
                  .then(async (response) => {
                      assert.equal(response.status(), 200);
                      return (await response.json()).data;
                  });
        await page.getByRole("button", { name: "发送", exact: true }).click();
        const task = await ack;
        assert.equal(task.status, "running");
        assert.ok(task.id && task.platformGenerationRequest);
        const initial = await counts();
        assert.equal(initial.posts, before.posts + 1);
        assert.equal(initial.gets, before.gets);
        if (!loseResponse) {
            await until(async () => (await counts()).gets > before.gets);
            assert.equal((await fetch(endpoint("/api/__fixture/previewable"), { method: "POST" })).status, 204);
            await until(async () => (await counts()).previewReads > before.previewReads && (await page.locator('img[src*="/previews/"]').evaluateAll((images) => images.some((img) => img.complete && img.naturalWidth === 16))));
            assert.equal((await counts()).adopted, false);
        }
        await page.close();
        release();
        await sleep(500);
        const closed = await counts();
        await sleep(1850);
        assert.deepEqual(await counts(), closed);
        assert.equal((await fetch(endpoint("/api/__fixture/ready"), { method: "POST" })).status, 204);
        const reopened = await context.newPage();
        let byCommand = 0;
        reopened.on("response", (response) => {
            if (response.status() === 200 && new URL(response.url()).pathname === "/api/tasks/by-command/" + task.platformGenerationRequest) byCommand++;
        });
        await reopened.goto(endpoint("/create").href);
        await until(async () => (await counts()).adopted && (!loseResponse || byCommand > 0));
        await until(() =>
            reopened.evaluate(async (id) => {
                const { getActiveUserScope } = await import("/src/lib/user-scope.ts");
                const { readCreationConversationSnapshot } = await import("/src/services/creation-conversation-snapshot.ts");
                return (readCreationConversationSnapshot(getActiveUserScope()) || []).some((conversation) => conversation.messages.some((message) => message.taskIds?.includes(id) && message.status === "done"));
            }, task.id),
        );
        const recovered = await reopened.evaluate(async (id) => (await (await fetch("/api/tasks/" + id)).json()).data, task.id);
        assert.equal(recovered.platformGenerationRequest, task.platformGenerationRequest);
        assert.equal(recovered.status, "succeeded");
        const final = await counts();
        assert.equal(final.posts, before.posts + 1);
        assert.equal(final.quotes, before.quotes);
        assert.ok(final.gets > closed.gets);
        const diff = (a, b) => Object.fromEntries(["posts", "gets", "quotes", "ticketGets", "previewReads"].map((k) => [k, a[k] - b[k]]));
        return {
            scenario: loseResponse ? "closed-before-first-studio-ack" : "early-preview-close-reopen",
            initial: diff(initial, before),
            atClose: diff(closed, before),
            final: diff(final, before),
            closedObservationMs: 1850,
            byCommand,
            sameOriginal: true,
            originalAdopted: true,
        };
    } finally {
        await context.close();
    }
}

async function canvasScenario(browser) {
    const before = await counts();
    const context = await restrictedContext(browser, { viewport: { width: 1600, height: 1050 } });
    try {
        const page = await context.newPage();
        await page.goto(endpoint("/canvas").href);
        await page.getByRole("button", { name: "新建画布", exact: true }).first().click();
        await page.getByRole("button", { name: "添加节点", exact: true }).click();
        await page.getByText("图片", { exact: true }).last().click();
        const root = page.locator("[data-node-id]").first();
        await root.dblclick();
        const originalNode = await root.getAttribute("data-node-id");
        const prompt = page.locator("textarea:visible").first();
        await prompt.fill("Free local three-output canvas image");
        await page.getByRole("button", { name: /图像设置/ }).click();
        await page.getByText("3", { exact: true }).last().click();
        await prompt.click();
        await page.getByRole("button", { name: /图像设置：.*3 张/ }).waitFor();
        const receipt = page.waitForResponse((r) => r.request().method() === "POST" && new URL(r.url()).pathname === "/api/tasks");
        const generate = page.getByRole("button", { name: "生成", exact: true });
        await generate.focus();
        await generate.press("Enter");
        const response = await receipt;
        assert.equal(response.status(), 200);
        const task = (await response.json()).data;
        assert.ok(task.id && task.platformGenerationRequest);
        const projectURL = page.url();
        const projectId = new URL(projectURL).pathname.split("/").at(-1);
        const snapshot = (target) =>
            target.evaluate(
                async ({ id, request, project }) => {
                    const { useCanvasStore } = await import("/src/stores/canvas/use-canvas-store.ts");
                    const canvas = useCanvasStore.getState().projects.find((item) => item.id === project);
                    return (canvas?.nodes || [])
                        .filter((n) => n.metadata?.taskId === id && n.metadata?.platformOutputRequest === request)
                        .map((n) => ({ id: n.id, index: n.metadata.platformOutputIndex, key: n.metadata.storageKey || null }))
                        .sort((a, b) => a.index - b.index);
                },
                { id: task.id, request: task.platformGenerationRequest, project: projectId },
            );
        await until(async () => {
            const state = await snapshot(page);
            return state.length === 3 && !!state[0].key && !state[1].key && !!state[2].key;
        });
        const initial = await snapshot(page);
        assert.equal(initial[0].id, originalNode);
        assert.deepEqual(
            initial.map((n) => n.index),
            [0, 1, 2],
        );
        await until(async () => (await counts()).previewReads > before.previewReads);
        const partial = await counts();
        assert.equal(partial.posts - before.posts, 1);
        assert.equal(partial.requestedOutputs, 3);
        assert.deepEqual(partial.boundIndexes, [0, 2]);
        await page.close();
        await sleep(500);
        const closed = await counts();
        await sleep(1850);
        assert.deepEqual(await counts(), closed);
        await fetch(endpoint("/api/__fixture/ready"), { method: "POST" });
        const reopened = await context.newPage();
        await reopened.goto(projectURL);
        await until(async () => {
            const state = await snapshot(reopened);
            return state.length === 3 && state.every((n) => n.key);
        });
        const final = await snapshot(reopened);
        assert.deepEqual(
            final.map((n) => n.id),
            initial.map((n) => n.id),
        );
        assert.equal(initial[0].key, final[0].key);
        assert.equal(initial[2].key, final[2].key);
        await reopened.reload();
        await until(async () => JSON.stringify(await snapshot(reopened)) === JSON.stringify(final));
        const end = await counts();
        assert.equal(end.posts - before.posts, 1);
        assert.equal(end.quotes - before.quotes, 0);
        assert.equal(end.bindingPosts - before.bindingPosts, 3);
        assert.deepEqual(end.boundIndexes, [0, 1, 2]);
        return {
            scenario: "canvas-three-outputs-close-reopen",
            requestedOutputs: 3,
            originalNodes: 3,
            initialBound: partial.boundIndexes,
            finalBound: end.boundIndexes,
            unchangedOnReplay: true,
            posts: end.posts - before.posts,
            gets: end.gets - before.gets,
            bindingPosts: end.bindingPosts - before.bindingPosts,
            quotes: 0,
            actualEarlyPreviewReads: partial.previewReads - before.previewReads,
            closedObservationMs: 1850,
        };
    } finally {
        await context.close();
    }
}

async function videoScenario(browser) {
    const before = await counts();
    const context = await restrictedContext(browser);
    try {
        const page = await context.newPage();
        await page.goto(endpoint("/create").href);
        await page.getByRole("button", { name: "视频生成", exact: true }).click();
        await page.getByRole("button", { name: "选择视频模型" }).getByText("Fixture Video").waitFor();
        await page.locator(".creation-chat-mention-editor:visible").first().fill("Free local video preview");
        const receipt = page.waitForResponse((r) => r.request().method() === "POST" && new URL(r.url()).pathname === "/api/tasks");
        await page.getByRole("button", { name: "发送", exact: true }).click();
        const ack = await receipt;
        assert.equal(ack.status(), 200);
        const task = (await ack.json()).data;
        await fetch(endpoint("/api/__fixture/previewable"), { method: "POST" });
        const video = page.locator('video[src*="/previews/"]').first();
        await video.waitFor();
        await until(() => video.evaluate((v) => v.videoWidth === 64 && v.videoHeight === 48 && v.duration > 1));
        await video.evaluate(async (v) => {
            v.dataset.originalPlaying = "1";
            v.muted = true;
            v.loop = true;
            v.playbackRate = 0.5;
            v.currentTime = 0.55;
            await v.play();
        });
        await until(() => video.evaluate((v) => !v.paused && v.currentTime > 0.6));
        assert.equal((await counts()).adopted, false);
        await fetch(endpoint("/api/__fixture/ready"), { method: "POST" });
        await until(async () => (await counts()).adopted);
        await until(() =>
            page.evaluate(async (id) => {
                const { getActiveUserScope } = await import("/src/lib/user-scope.ts");
                const { readCreationConversationSnapshot } = await import("/src/services/creation-conversation-snapshot.ts");
                return (readCreationConversationSnapshot(getActiveUserScope()) || []).some((c) => c.messages.some((m) => m.taskIds?.includes(id) && m.status === "done"));
            }, task.id),
        );
        const playing = page.locator('video[data-original-playing="1"]');
        assert.equal(await playing.count(), 1);
        assert.ok(await playing.evaluate((v) => !v.paused && v.videoWidth === 64 && v.duration > 1));
        const final = await counts();
        assert.ok(final.rangeReads > before.rangeReads);
        assert.equal(final.posts - before.posts, 1);
        assert.equal(final.quotes - before.quotes, 0);
        return { scenario: "early-video-across-original-adoption", width: 64, height: 48, seekSeconds: 0.55, rangeReads: final.rangeReads - before.rangeReads, posts: 1, quotes: 0, samePlayingElement: true };
    } finally {
        await context.close();
    }
}

(async () => {
    const browser = await chromium.launch({ executablePath: process.env.STUDIO_CHROMIUM, headless: true, args: ["--disable-background-networking"] });
    try {
        const scenarios = [await scenario(browser, false), await scenario(browser, true), await canvasScenario(browser), await videoScenario(browser)];
        process.stdout.write(JSON.stringify({ chrome: browser.version(), scenarios }, null, 2) + "\n");
    } finally {
        await browser.close();
    }
})().catch((error) => {
    console.error(error.stack);
    process.exitCode = 1;
});
