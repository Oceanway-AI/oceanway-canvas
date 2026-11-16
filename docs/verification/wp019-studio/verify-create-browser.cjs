const assert = require('node:assert/strict');
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || 'playwright');
const origin = new URL(process.env.STUDIO_MEDIA_BROWSER_ORIGIN);
assert.equal(origin.hostname, '127.0.0.1');
const endpoint = path => new URL(path, origin);
const counts = async () => (await fetch(endpoint('/api/__fixture/counts'))).json();
const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));

async function until(check) {
    for (let attempt = 0; attempt < 100; attempt++) {
        if (await check()) return;
        await sleep(150);
    }
    throw Error('The expected browser observation did not arrive');
}

async function scenario(browser, loseResponse) {
    const before = await counts();
    const context = await browser.newContext();
    try {
        await context.route('**/*', route => {
            const url = new URL(route.request().url());
            const allowed = url.origin === origin.origin || (url.hostname === '127.0.0.1' && url.pathname.startsWith('/api/platform/v1/media-previews/'));
            return /^https?:/.test(url.protocol) && !allowed ? route.abort() : route.continue();
        });
        const page = await context.newPage();
        await page.goto(endpoint('/create').href);
        await page.getByRole('button', { name: '选择图片模型' }).getByText('Fixture Image').waitFor();
        await page.locator('.creation-chat-mention-editor:visible').first().fill('Free local original media frame');
        let receive, release;
        const accepted = new Promise(resolve => { receive = resolve; });
        const blocked = new Promise(resolve => { release = resolve; });
        if (loseResponse) {
            await page.route('**/api/tasks', async route => {
                if (route.request().method() !== 'POST') return route.continue();
                const response = await route.fetch();
                assert.equal(response.status(), 200);
                receive((await response.json()).data);
                await blocked;
            });
        }
        const ack = loseResponse ? accepted : page.waitForResponse(response => response.request().method() === 'POST' && new URL(response.url()).pathname === '/api/tasks').then(async response => {
            assert.equal(response.status(), 200);
            return (await response.json()).data;
        });
        await page.getByRole('button', { name: '发送', exact: true }).click();
        const task = await ack;
        assert.equal(task.status, 'running');
        assert.ok(task.id && task.platformGenerationRequest);
        const initial = await counts();
        assert.equal(initial.posts, before.posts + 1);
        assert.equal(initial.gets, before.gets);
        if (!loseResponse) {
            await until(async () => (await counts()).gets > before.gets);
            assert.equal((await fetch(endpoint('/api/__fixture/previewable'), { method: 'POST' })).status, 204);
            await until(async () => (await counts()).previewReads > before.previewReads && await page.locator('img[src*="/previews/"]').evaluateAll(images => images.some(img => img.complete && img.naturalWidth === 16)));
            assert.equal((await counts()).adopted, false);
        }
        await page.close();
        release();
        await sleep(500);
        const closed = await counts();
        await sleep(1850);
        assert.deepEqual(await counts(), closed);
        assert.equal((await fetch(endpoint('/api/__fixture/ready'), { method: 'POST' })).status, 204);
        const reopened = await context.newPage();
        let byCommand = 0;
        reopened.on('response', response => {
            if (response.status() === 200 && new URL(response.url()).pathname === '/api/tasks/by-command/' + task.platformGenerationRequest) byCommand++;
        });
        await reopened.goto(endpoint('/create').href);
        await until(async () => (await counts()).adopted && (!loseResponse || byCommand > 0));
        await until(() => reopened.evaluate(async id => {
            const { getActiveUserScope } = await import('/src/lib/user-scope.ts');
            const { readCreationConversationSnapshot } = await import('/src/services/creation-conversation-snapshot.ts');
            return (readCreationConversationSnapshot(getActiveUserScope()) || []).some(conversation => conversation.messages.some(message => message.taskIds?.includes(id) && message.status === 'done'));
        }, task.id));
        const recovered = await reopened.evaluate(async id => (await (await fetch('/api/tasks/' + id)).json()).data, task.id);
        assert.equal(recovered.platformGenerationRequest, task.platformGenerationRequest);
        assert.equal(recovered.status, 'succeeded');
        const final = await counts();
        assert.equal(final.posts, before.posts + 1);
        assert.equal(final.quotes, before.quotes);
        assert.ok(final.gets > closed.gets);
        const diff = (a, b) => Object.fromEntries(['posts','gets','quotes','ticketGets','previewReads'].map(k => [k,a[k] - b[k]]));
        return { scenario: loseResponse ? 'closed-before-first-studio-ack' : 'early-preview-close-reopen', initial: diff(initial,before), atClose: diff(closed,before), final: diff(final,before), closedObservationMs: 1850, byCommand, sameOriginal:true, originalAdopted:true };
    } finally {
        await context.close();
    }
}

(async () => {
    const browser = await chromium.launch({ executablePath: process.env.STUDIO_CHROMIUM, headless:true, args:['--disable-background-networking'] });
    try {
        const scenarios = [await scenario(browser,false), await scenario(browser,true)];
        process.stdout.write(JSON.stringify({chrome:browser.version(),scenarios},null,2)+'\n');
    } finally {
        await browser.close();
    }
})().catch(error => { console.error(error.message); process.exitCode = 1; });
