/* Run only in the coordinated W7 upload window. One new free upload, original UI and routes. */
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const crypto = require("node:crypto");
const { inspect } = require("node:util");
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || "playwright");

const canvasURL = new URL(process.env.STUDIO_CANVAS_URL);
assert.equal(canvasURL.origin, "https://test.oceanway.tech:18448");
assert.match(canvasURL.pathname, /^\/canvas\/[^/]+$/);
const canvasId = decodeURIComponent(canvasURL.pathname.split("/").at(-1));
const fixture = fs.readFileSync(process.env.UPLOAD_PNG);
assert.equal(fixture.length, 34621850);
assert.equal(crypto.createHash("sha256").update(fixture).digest("hex"), "6169b16d719435bfa8d9b506c00461bb32c0f96d3ac810c7b38b834df47117c0");
const output = process.env.UPLOAD_EVIDENCE_DIR;
fs.mkdirSync(output, { recursive: true });
const priorEvidencePath = path.join(output, "studio-browser-recovery.json");
if (fs.existsSync(priorEvidencePath)) {
    const previous = JSON.parse(fs.readFileSync(priorEvidencePath, "utf8"));
    assert.equal(previous.creates.length, 0, "This output already has an original upload. Continue its saved node and private session.");
}
const privateOutput = path.join(output, "private");
fs.mkdirSync(privateOutput, { recursive: true, mode: 0o700 });
fs.chmodSync(privateOutput, 0o700);
const privateDiagnostics = [];
const privateError = (stage, error) => privateDiagnostics.push({ at: new Date().toISOString(), stage, detail: inspect(error, { depth: 5 }) });
const evidence = { startedAt: new Date().toISOString(), canvasId, fixture: { byteLength: fixture.length, sha256: "6169b16d719435bfa8d9b506c00461bb32c0f96d3ac810c7b38b834df47117c0", width: 4096, height: 2816 }, creates: [], statusReads: [], completes: [], puts: [], failedPuts: [], browserClosure: false, result: "in_progress" };
const save = () => fs.writeFileSync(path.join(output, "studio-browser-recovery.json"), JSON.stringify(evidence, null, 2) + "\n");
const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
async function until(check, label, timeout = 180000) {
    const deadline = Date.now() + timeout;
    while (Date.now() < deadline) { const result = await check(); if (result) return result; await sleep(300); }
    throw new Error(label);
}

(async () => {
    const browser = await chromium.launch({ executablePath: process.env.STUDIO_CHROMIUM || undefined, headless: true });
    const context = await browser.newContext({ storageState: process.env.STUDIO_STORAGE_STATE, viewport: { width: 1600, height: 1050 } });
    const preserveSession = async () => {
        const statePath = path.join(privateOutput, "resume-storage-state.json");
        await context.storageState({ path: statePath, indexedDB: true });
        fs.chmodSync(statePath, 0o600);
    };
    const tickets = new Map();
    let initialPage;
    let blockedSecond;
    let releaseSecond;
    let nodeId;
    let hostUploadId;
    let platformUploadId;
    let readyResource;
    let phase = "initial";
    const observed = [];
    const pendingResponses = new Set();
    const requestPhases = new WeakMap();
    context.on("request", (request) => requestPhases.set(request, phase));
    context.on("requestfailed", (request) => {
        privateDiagnostics.push({ at: new Date().toISOString(), stage: "requestfailed", method: request.method(), url: request.url(), failure: request.failure() });
        if (request.method() === "PUT" && tickets.has(request.url())) {
            evidence.failedPuts.push({ phase: requestPhases.get(request), part: tickets.get(request.url()), at: new Date().toISOString() });
            save();
        }
    });
    context.on("page", (page) => page.on("pageerror", (error) => privateError("pageerror", error)));
    context.on("response", (response) => {
        const observe = (async () => {
            const request = response.request();
            const requestPhase = requestPhases.get(request) || phase;
            const url = new URL(response.url());
            if (url.origin === canvasURL.origin && url.pathname === "/api/resources/adopt" && request.method() === "POST") {
                const body = await response.json();
                const resource = body.data?.resource;
                if (body.code === 0 && resource?.status === "ready") readyResource = { id: resource.id, width: resource.width, height: resource.height };
            }
            if (url.origin === canvasURL.origin && url.pathname.startsWith("/api/resources/platform-uploads")) {
                const body = await response.json();
                if (body.code !== 0) return;
                const data = body.data;
                assert.ok(data.id && data.upload?.upload_id);
                if (hostUploadId) assert.equal(data.id, hostUploadId);
                if (platformUploadId) assert.equal(data.upload.upload_id, platformUploadId);
                hostUploadId = data.id;
                platformUploadId = data.upload.upload_id;
                for (const ticket of data.upload.tickets || []) tickets.set(ticket.url, ticket.part_number);
                const safe = { phase: requestPhase, hostUploadId, platformUploadId, state: data.upload.state, status: response.status() };
                if (request.method() === "POST" && url.pathname === "/api/resources/platform-uploads") evidence.creates.push(safe);
                if (request.method() === "GET") evidence.statusReads.push(safe);
                if (url.pathname.endsWith("/complete")) evidence.completes.push(safe);
                if (data.resource?.status === "ready") readyResource = { id: data.resource.id, width: data.resource.width, height: data.resource.height };
                save();
            }
            if (request.method() === "PUT" && tickets.has(request.url())) {
                evidence.puts.push({ phase: requestPhase, part: tickets.get(request.url()), status: response.status(), byteLength: request.postDataBuffer()?.length || 0, acknowledged: Boolean(response.headers()["etag"]), at: new Date().toISOString() });
                save();
            }
        })();
        pendingResponses.add(observe);
        observe.catch((error) => { observed.push(error.name); privateError("response-observer", error); }).finally(() => pendingResponses.delete(observe));
    });
    try {
        const healthResponse = await context.request.get(new URL("/api/health/ready", canvasURL.origin).href);
        const health = (await healthResponse.json()).data;
        assert.equal(health.ready, true);
        assert.equal(health.build.commit, "8274f1d0841760b3e66916d5921de53f04fc0831");
        evidence.runtime = { studio: health.build.commit, schema: health.schema.current };
        const beforeStorage = await context.request.get(new URL("/api/materials/storage", canvasURL.origin).href);
        const beforeStorageBody = await beforeStorage.json();
        assert.equal(beforeStorageBody.code, 0);
        evidence.storageBefore = { usedBytes: beforeStorageBody.data.used_bytes, reservedBytes: beforeStorageBody.data.reserved_bytes };
        initialPage = await context.newPage();
        await initialPage.goto(canvasURL.href, { waitUntil: "domcontentloaded" });
        const initialInputs = initialPage.locator('input[type="file"][accept="image/*,video/*,audio/mpeg,audio/wav,audio/x-wav,.mp3,.wav"]');
        await initialInputs.waitFor({ state: "attached" });
        const beforeNodes = await initialPage.locator("[data-node-id]").evaluateAll((elements) => elements.map((node) => node.getAttribute("data-node-id")));
        const originalCanvas = await initialPage.request.get(new URL(`/api/canvas-projects/${encodeURIComponent(canvasId)}`, canvasURL.origin).href);
        const originalCanvasBody = await originalCanvas.json();
        assert.equal(originalCanvasBody.code, 0);
        evidence.existingNodes = originalCanvasBody.data.project.nodes.map((node) => ({ id: node.id, type: node.type, taskId: node.metadata?.taskId || null, storageKey: node.metadata?.storageKey || null }));
        save();
        // Pause only part 2 before it is sent, after the real part-1 acknowledgement.
        await initialPage.route("**/*", async (route) => {
            const request = route.request();
            if (request.method() !== "PUT" || new URL(request.url()).origin === canvasURL.origin) return route.continue();
            await until(() => tickets.has(request.url()), "ticket mapping missing", 5000);
            if (tickets.get(request.url()) !== 2) return route.continue();
            blockedSecond = true;
            await new Promise((resolve) => { releaseSecond = resolve; });
            await route.abort().catch(() => {});
        });
        await initialInputs.setInputFiles(process.env.UPLOAD_PNG);
        await until(() => blockedSecond && evidence.puts.some((put) => put.part === 1 && put.status === 200 && put.acknowledged), "first part not acknowledged");
        nodeId = await until(async () => (await initialPage.locator("[data-node-id]").evaluateAll((elements) => elements.map((node) => node.getAttribute("data-node-id")))).find((id) => !beforeNodes.includes(id)), "placeholder missing");
        evidence.nodeId = nodeId;
        evidence.beforeClose = await initialPage.evaluate((id) => {
            const entry = Object.keys(localStorage).filter((key) => key.startsWith("studio-material-upload:v1:")).map((key) => JSON.parse(localStorage.getItem(key))).find((item) => item.id === id);
            return entry && { id: entry.id, uploadId: entry.uploadId, confirmedParts: entry.parts.map((part) => part.part_number), partSize: entry.partSize };
        }, hostUploadId);
        assert.deepEqual(evidence.beforeClose.confirmedParts, [1]);
        await initialPage.screenshot({ path: path.join(output, "studio-before-close.png") });
        await preserveSession();
        await initialPage.close();
        releaseSecond();
        evidence.browserClosure = true;
        phase = "reopened";
        save();
        const reopened = await context.newPage();
        await reopened.goto(canvasURL.href, { waitUntil: "domcontentloaded" });
        const node = reopened.locator(`[data-node-id="${nodeId}"]`);
        const retry = node.getByRole("button", { name: /重新选择文件|Choose.*file|Select.*file/i });
        await retry.waitFor();
        await reopened.screenshot({ path: path.join(output, "studio-reopened-pending.png") });
        const chooser = reopened.waitForEvent("filechooser");
        await retry.click();
        await (await chooser).setFiles(process.env.UPLOAD_PNG);
        await until(() => {
            if (evidence.failedPuts.some((item) => item.phase === "reopened") || evidence.puts.some((item) => item.phase === "reopened" && item.status >= 400)) throw new Error("reopened upload transport failed; continue its original node and saved session");
            return readyResource;
        }, "upload not ready", 1800000);
        await until(async () => {
            const response = await reopened.request.get(new URL(`/api/canvas-projects/${encodeURIComponent(canvasId)}`, canvasURL.origin).href);
            const body = await response.json();
            const persisted = body.data?.project?.nodes?.find((item) => item.id === nodeId);
            if (persisted?.metadata?.storageKey !== `resource:${readyResource.id}`) return false;
            const previousIds = new Set(evidence.existingNodes.map((item) => item.id));
            assert.deepEqual(body.data.project.nodes.filter((item) => previousIds.has(item.id)).map((item) => ({ id: item.id, type: item.type, taskId: item.metadata?.taskId || null, storageKey: item.metadata?.storageKey || null })), evidence.existingNodes);
            evidence.existingNodesPreserved = true;
            evidence.finalNode = { id: persisted.id, storageKey: persisted.metadata.storageKey, naturalWidth: persisted.metadata.naturalWidth, naturalHeight: persisted.metadata.naturalHeight, retainedUploadIdentity: Boolean(persisted.metadata.fileUploadIdentity), fileUpload: persisted.metadata.fileUpload || null };
            return true;
        }, "canvas result not persisted");
        await Promise.all([...pendingResponses]);
        assert.equal(evidence.creates.length, 1);
        assert.ok(evidence.statusReads.some((read) => read.phase === "reopened"));
        assert.deepEqual(evidence.puts.filter((put) => put.phase === "reopened").map((put) => put.part), [2, 3, 4, 5]);
        assert.equal(evidence.finalNode.fileUpload, null);
        assert.equal(evidence.finalNode.naturalWidth, 4096);
        assert.equal(evidence.finalNode.naturalHeight, 2816);
        const original = await reopened.request.get(new URL(`/api/resources/${readyResource.id}/file`, canvasURL.origin).href, { timeout: 180000 });
        assert.equal(original.status(), 200);
        const bytes = await original.body();
        evidence.download = { bytes: bytes.length, sha256: crypto.createHash("sha256").update(bytes).digest("hex") };
        assert.equal(evidence.download.sha256, evidence.fixture.sha256);
        assert.equal(bytes.length, fixture.length);
        const afterStorage = await context.request.get(new URL("/api/materials/storage", canvasURL.origin).href);
        const afterStorageBody = await afterStorage.json();
        assert.equal(afterStorageBody.code, 0);
        evidence.storageAfter = { usedBytes: afterStorageBody.data.used_bytes, reservedBytes: afterStorageBody.data.reserved_bytes };
        assert.equal(BigInt(evidence.storageAfter.usedBytes) - BigInt(evidence.storageBefore.usedBytes), BigInt(fixture.length));
        assert.equal(evidence.storageAfter.reservedBytes, evidence.storageBefore.reservedBytes);
        await reopened.screenshot({ path: path.join(output, "studio-recovered-ready.png") });
        assert.deepEqual(observed, []);
        evidence.result = "passed";
        evidence.completedAt = new Date().toISOString();
        save();
    } catch (error) {
        evidence.result = "failed";
        evidence.errorType = error.name;
        // Error strings and request objects may contain signed tickets, so keep them private.
        privateError("browser-flow", error);
        save();
        process.exitCode = 1;
    } finally {
        releaseSecond?.();
        try { await preserveSession(); } catch (error) { privateError("preserve-session", error); }
        if (evidence.result === "failed") {
            const currentPage = context.pages().find((page) => !page.isClosed());
            if (currentPage) {
                try { await currentPage.screenshot({ path: path.join(privateOutput, "failure.png") }); } catch (error) { privateError("failure-screenshot", error); }
            }
        }
        fs.writeFileSync(path.join(privateOutput, "diagnostics.json"), JSON.stringify({ phase, canvasId, nodeId, hostUploadId, platformUploadId, diagnostics: privateDiagnostics }, null, 2) + "\n", { mode: 0o600 });
        await context.close().catch((error) => privateError("context-close", error));
        await browser.close().catch((error) => privateError("browser-close", error));
    }
})();
