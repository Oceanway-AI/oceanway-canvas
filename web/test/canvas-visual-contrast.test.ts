import { oceanAirPalette } from "../src/lib/ocean-air-theme";
import { describe, expect, test } from "bun:test";

import { canvasThemes } from "../src/lib/canvas-theme";

describe("canvas visual contrast", () => {
    test("keeps the canvas substrate distinct from node surfaces in both themes", () => {
        for (const theme of Object.values(canvasThemes)) {
            expect(theme.canvas.background).not.toBe(theme.node.fill);
            expect(theme.node.panel).not.toBe(theme.canvas.background);
        }
    });

    test("uses the Ocean Air palette while retaining node contours and shadows", () => {
        expect(canvasThemes.light.canvas.background).toBe(oceanAirPalette.light.background);
        expect(canvasThemes.light.node.fill).toBe(oceanAirPalette.light.surface);
        expect(canvasThemes.light.node.edge).toBe("rgba(15,23,42,.16)");
        expect(canvasThemes.light.node.shadow).toBe("0 6px 18px rgba(15,23,42,.08)");
        expect(canvasThemes.light.node.hoverShadow).toBe("0 10px 24px rgba(15,23,42,.12)");
        expect(canvasThemes.light.toolbar.panel).toBe("rgba(248,250,252,.94)");
        expect(canvasThemes.light.spatial.elevated).toBe("rgba(255,255,255,.94)");

        expect(canvasThemes.dark.canvas.background).toBe(oceanAirPalette.dark.background);
        expect(canvasThemes.dark.node.fill).toBe(oceanAirPalette.dark.surface);
        expect(canvasThemes.dark.node.edge).toBe("rgba(255,255,255,.18)");
        expect(canvasThemes.dark.node.shadow).toBe("0 8px 24px rgba(0,0,0,.34)");
        expect(canvasThemes.dark.node.hoverShadow).toBe("0 12px 30px rgba(0,0,0,.46)");
        expect(canvasThemes.dark.toolbar.panel).toBe("rgba(26,32,40,.97)");
        expect(canvasThemes.dark.toolbar.border).toBe(oceanAirPalette.dark.border);
        expect(canvasThemes.dark.spatial.elevated).toBe("rgba(26,32,40,.97)");
    });

    test("pins intentional grid tokens while retaining canvas grid opacity", async () => {
        expect(canvasThemes.light.canvas.dot).toBe("rgba(46,61,78,.23)");
        expect(canvasThemes.light.canvas.line).toBe("rgba(46,61,78,.23)");
        expect(canvasThemes.dark.canvas.dot).toBe("rgba(232,241,250,.24)");
        expect(canvasThemes.dark.canvas.line).toBe("rgba(232,241,250,.24)");

        const source = await Bun.file(new URL("../src/components/canvas/canvas-viewport.tsx", import.meta.url)).text();
        expect(source).toContain('opacity: mode === "dots" ? 0.62 : 0.4');
    });

    test("keeps the original transparent edge for standard canvas nodes", async () => {
        const source = await Bun.file(new URL("../src/components/canvas/canvas-node.tsx", import.meta.url)).text();

        expect(source).toContain('border: isComposerNode ? "0" : "1px solid transparent"');
        expect(source).not.toContain('border: isComposerNode ? "0" : `1px solid ${theme.node.edge}`');
    });
});
