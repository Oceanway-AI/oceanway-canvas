import { expect, test } from "bun:test";

import { createMoltenMetalContext } from "../src/components/ocean/backgrounds/molten-metal";

function browserCanvas(renderer: string | null, available = true) {
    let released = false;
    const gl = {
        getExtension(name: string) {
            if (name === "WEBGL_debug_renderer_info" && renderer) return { UNMASKED_RENDERER_WEBGL: 37446 };
            if (name === "WEBGL_lose_context") return { loseContext: () => (released = true) };
            return null;
        },
        getParameter: () => renderer,
    };
    const requests: { type: string; attributes?: WebGLContextAttributes }[] = [];
    const canvas = {
        getContext(type: string, attributes?: WebGLContextAttributes) {
            requests.push({ type, attributes });
            return available ? gl : null;
        },
    } as HTMLCanvasElement;
    return { canvas, gl, requests, released: () => released };
}

test("the decorative background keeps its CSS surface on software or unavailable WebGL", () => {
    for (const name of ["ANGLE (Vulkan 1.3, SwiftShader Device (Subzero))", "llvmpipe (LLVM 19)", "Microsoft Basic Render Driver"]) {
        const browser = browserCanvas(name);
        expect(createMoltenMetalContext(browser.canvas)).toBeNull();
        expect(browser.released()).toBe(true);
        expect(browser.requests).toHaveLength(1);
        expect(browser.requests[0]?.attributes?.failIfMajorPerformanceCaveat).toBe(true);
    }
    const unavailable = browserCanvas(null, false);
    expect(createMoltenMetalContext(unavailable.canvas)).toBeNull();
    expect(unavailable.requests.map((request) => request.type)).toEqual(["webgl2"]);
});

test("the background reuses the available hardware context even with renderer details withheld", () => {
    for (const name of ["ANGLE (Apple, Apple M2, Metal)", "Mesa Intel(R) Graphics", null]) {
        const browser = browserCanvas(name);
        expect(createMoltenMetalContext(browser.canvas)).toBe(browser.gl);
        expect(browser.released()).toBe(false);
        expect(browser.requests).toHaveLength(1);
    }
});
