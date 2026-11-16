import { expect, test } from "bun:test";
import { appBasePath, appRoute, appURL } from "../src/lib/app-path";
import { platformLoginNext } from "../src/lib/platform-login";
import { apiBaseURL } from "../src/services/api/request";
import { isResourceUrl, resolveResourceAccessURL } from "../src/services/api/resources";
import { canvasRealtimeURL } from "../src/services/canvas-realtime-client";

test("browser URLs use the configured application path exactly once", () => {
    expect(appURL("/api/resources/file")).toBe(`${appBasePath}/api/resources/file`);
    expect(appURL("/oceanway/logo.png")).toBe(`${appBasePath}/oceanway/logo.png`);
    expect(appURL(`${appBasePath}/canvas/123`)).toBe(`${appBasePath}/canvas/123`);
    for (const url of ["https://cdn.example/file", "//cdn.example/file", "data:image/png;base64,fixture", "blob:fixture"]) {
        expect(appURL(url)).toBe(url);
    }
    if (appBasePath) {
        expect(appURL(`${appBasePath}?tab=one`)).toBe(`${appBasePath}?tab=one`);
        expect(appURL(`${appBasePath}#canvas`)).toBe(`${appBasePath}#canvas`);
        expect(appRoute(`${appBasePath}?tab=one`)).toBe("/?tab=one");
        expect(appRoute(`${appBasePath}#canvas`)).toBe("/#canvas");
        expect(appRoute(`${appBasePath}ish/canvas`)).toBe(`${appBasePath}ish/canvas`);
    }
});

test("login restores a route within the application basename", () => {
    expect(platformLoginNext(`${appBasePath}/canvas/123?tab=details`)).toBe("/canvas/123?tab=details");
    expect(platformLoginNext(`${appBasePath}/platform/login/complete`)).toBe("/");
    expect(appRoute(`${appBasePath}/`)).toBe("/");
});

test("existing resource URLs and realtime use the configured browser API", () => {
    expect(resolveResourceAccessURL("/api/resources/old-id/file?variant=preview")).toBe(`${apiBaseURL}/resources/old-id/file?variant=preview`);
    expect(isResourceUrl("/api/resources/old-id/file")).toBe(true);
    expect(resolveResourceAccessURL(`${apiBaseURL}/resources/new-id/file`)).toBe(`${apiBaseURL}/resources/new-id/file`);
    expect(resolveResourceAccessURL("https://cdn.example/file?sign=fixture")).toBe("https://cdn.example/file?sign=fixture");
    expect(canvasRealtimeURL(apiBaseURL, "canvas-id", `https://test.oceanway.tech${appBasePath}/canvas/canvas-id`)).toBe(`wss://test.oceanway.tech${apiBaseURL}/canvas-projects/canvas-id/realtime`);
});
