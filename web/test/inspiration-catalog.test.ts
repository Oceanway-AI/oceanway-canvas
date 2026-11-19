import { expect, test } from "bun:test";
import { invalidateInspirationCatalog, loadInspirationCatalog } from "../src/services/inspiration-catalog";

test("anonymous inspiration catalog loads static packs without retired API requests", async () => {
    const originalFetch = globalThis.fetch;
    const requests: string[] = [];
    globalThis.fetch = (async (input: RequestInfo | URL) => {
        const url = String(input);
        requests.push(url);
        if (url.endsWith("manifest.json")) return Response.json({ sources: { seedance: { categories: { film: "电影" } } } });
        const source = url.split("/").at(-1)!.replace(".json", "");
        return Response.json({ entries: [{ id: `${source}:sample`, sourceId: source, title: "示例", prompt: "示例提示词", image: "/cover.webp", category: "film", mode: "image" }] });
    }) as typeof fetch;
    invalidateInspirationCatalog();
    try {
        const catalog = await loadInspirationCatalog();
        expect(requests).toHaveLength(4);
        expect(requests.every((url) => url.startsWith("/inspirations/") && url.endsWith(".json"))).toBe(true);
        expect(catalog.entries.filter((entry) => entry.id?.endsWith(":sample"))).toHaveLength(3);
        expect(catalog.categoriesBySource.seedance).toEqual([{ id: "film", label: "电影", count: 1 }]);
        expect(await loadInspirationCatalog()).toBe(catalog);
        expect(requests).toHaveLength(4);
    } finally {
        globalThis.fetch = originalFetch;
        invalidateInspirationCatalog();
    }
});
