"use client";

/*
 * React Bits · MoltenMetal
 * https://github.com/DavidHDev/react-bits/blob/1ba7ecd4e570d20392fb62c923efb00f5b440642/src/ts-default/Backgrounds/MoltenMetal/MoltenMetal.tsx
 *
 * This is an application-owned adaptation of the pinned upstream component.
 * The shader remains close to React Bits, while lifecycle, accessibility,
 * visibility and fallback behavior are owned by OceanWay.
 */

import { useEffect, useRef } from "react";
import { Mesh, Program, Renderer, Triangle } from "ogl";

import { oceanAirPalette } from "@/lib/ocean-air-theme";

import styles from "./molten-metal.module.css";

export type MoltenMetalColorMode = "molten" | "ember" | "frost";

type MoltenMetalContext = {
    renderer: Renderer;
    program: Program;
    mesh: Mesh;
};

type MoltenMetalContextFallbackReason = "lost" | "restored";

/**
 * A lost WebGL context must never trigger a second renderer during the same
 * mount. Keep this transition independent from OGL so it can be regression
 * tested without constructing a browser GPU context.
 */
export function observeMoltenMetalContextLoss(target: EventTarget, onFallback: (reason: MoltenMetalContextFallbackReason) => void) {
    let state: "available" | MoltenMetalContextFallbackReason = "available";
    const handleContextLost = (event: Event) => {
        event.preventDefault();
        if (state !== "available") return;
        state = "lost";
        onFallback("lost");
    };
    const handleContextRestored = () => {
        if (state !== "lost") return;
        state = "restored";
        onFallback("restored");
    };

    target.addEventListener("webglcontextlost", handleContextLost, { passive: false });
    target.addEventListener("webglcontextrestored", handleContextRestored);

    return () => {
        target.removeEventListener("webglcontextlost", handleContextLost);
        target.removeEventListener("webglcontextrestored", handleContextRestored);
    };
}

export type MoltenMetalProps = {
    color1?: string;
    color2?: string;
    color3?: string;
    speed?: number;
    scale?: number;
    detail?: number;
    glow?: number;
    coreSize?: number;
    swirl?: number;
    fold?: number;
    blackPoint?: number;
    brightness?: number;
    colorMode?: MoltenMetalColorMode;
    grain?: boolean;
    grainIntensity?: number;
    mouseInteraction?: boolean;
    mouseStrength?: number;
    opacity?: number;
    className?: string;
};

const hexToRgb = (hex: string): [number, number, number] => {
    const normalized = typeof hex === "string" ? hex.trim() : "";
    const result = /^#?([a-f\d]{2})([a-f\d]{2})([a-f\d]{2})$/i.exec(normalized);
    if (!result) return [1, 1, 1];
    return [parseInt(result[1], 16) / 255, parseInt(result[2], 16) / 255, parseInt(result[3], 16) / 255];
};

const finiteClamp = (value: number, fallback: number, min: number, max: number) => (Number.isFinite(value) ? Math.min(max, Math.max(min, value)) : fallback);

const colorModeToFloat = (mode: MoltenMetalColorMode) => (mode === "ember" ? 1 : mode === "frost" ? 2 : 0);

const getMediaPreference = (query: string): MediaQueryList => {
    if (typeof window.matchMedia === "function") return window.matchMedia(query);
    return {
        matches: false,
        media: query,
        onchange: null,
        addEventListener: () => undefined,
        removeEventListener: () => undefined,
        addListener: () => undefined,
        removeListener: () => undefined,
        dispatchEvent: () => false,
    } as unknown as MediaQueryList;
};

const vertex = `#version 300 es
in vec2 position;
void main() {
  gl_Position = vec4(position, 0.0, 1.0);
}
`;

const fragment = `#version 300 es
precision highp float;
uniform vec2 iResolution;
uniform float iTime;
uniform float uSpeed;
uniform float uScale;
uniform float uDetail;
uniform float uGlow;
uniform float uCoreSize;
uniform float uSwirl;
uniform float uFold;
uniform float uBlackPoint;
uniform float uBrightness;
uniform float uColorMode;
uniform float uGrain;
uniform float uGrainIntensity;
uniform float uOpacity;
uniform vec2 uMouse;
uniform float uMouseStrength;
uniform bool uEnableMouse;
uniform vec3 uColor1;
uniform vec3 uColor2;
uniform vec3 uColor3;
out vec4 fragColor;

float hash(vec2 p) {
  return fract(sin(dot(p, vec2(12.9898, 78.233))) * 43758.5453);
}

void main() {
  float time = iTime * uSpeed;
  vec2 p = uScale * ((gl_FragCoord.xy - 0.5 * iResolution.xy) / iResolution.y) - 0.5;

  vec2 drift = vec2(0.0);
  if (uEnableMouse) {
    drift = (uMouse - 0.5) * uMouseStrength * 2.0;
  }
  p += drift;

  vec2 i = p;
  float c = 0.0;
  float r = length(p + vec2(sin(time), sin(time * 0.3 + 5.0)) * 0.5);
  float d = length(p);
  float rot = d + time + p.x * uSwirl;

  float cosRot = cos(rot);
  mat2 warp = mat2(cos(rot - sin(time / 5.0)), sin(rot), -sin(cosRot - time), cosRot) * uFold;
  float glowCore = uGlow * uCoreSize;

  for (float n = 0.0; n < 8.0; n++) {
    if (n >= uDetail) break;
    p *= warp;
    float t = r - time / (n + 3.0);
    i -= p + vec2(cos(t - i.x - r) + sin(t + i.y), sin(t - i.y) + cos(t + i.x) + r);
    c += glowCore / length(vec2(sin(i.x + t), cos(i.y + t)));
  }

  c /= 6.0;

  float intensity = max(c - uBlackPoint, 0.0) * uBrightness;

  float g = clamp(intensity, 0.0, 1.0);

  float mid = 0.5;
  if (uColorMode > 1.5) {
    mid = 0.65;
  } else if (uColorMode > 0.5) {
    mid = 0.35;
  }

  vec3 col = mix(uColor1, uColor2, smoothstep(0.0, mid, g));
  col = mix(col, uColor3, smoothstep(mid, 1.0, g));

  float a = g;
  if (uGrain > 0.5) {
    float gr = hash(gl_FragCoord.xy + iTime);
    a += (gr - 0.5) * uGrainIntensity;
  }
  a = clamp(a, 0.0, 1.0) * uOpacity;
  fragColor = vec4(col * a, a);
}
`;

const ctxMap = new WeakMap<HTMLDivElement, MoltenMetalContext>();

export function MoltenMetal({
    color1 = oceanAirPalette.light.primary,
    color2 = oceanAirPalette.light.sky,
    color3 = oceanAirPalette.light.surface,
    speed = 0.24,
    scale = 4,
    detail = 3,
    glow = 1.35,
    coreSize = 0.09,
    swirl = 0.9,
    fold = -0.2,
    blackPoint = 0.08,
    brightness = 1.05,
    colorMode = "frost",
    grain = false,
    grainIntensity = 0.02,
    mouseInteraction = true,
    mouseStrength = 0.18,
    opacity = 0.58,
    className = "",
}: MoltenMetalProps) {
    const containerRef = useRef<HTMLDivElement>(null);

    useEffect(() => {
        const container = containerRef.current;
        if (!container) return;

        container.dataset.contextStatus = "css-fallback";

        // Respect accessibility preferences before allocating a WebGL context.
        // The host's CSS fallback remains available when visual effects are off.
        const motionPreference = getMediaPreference("(prefers-reduced-motion: reduce)");
        const transparencyPreference = getMediaPreference("(prefers-reduced-transparency: reduce)");
        const forcedColorsPreference = getMediaPreference("(forced-colors: active)");
        if (motionPreference.matches || transparencyPreference.matches || forcedColorsPreference.matches) return;

        let renderer: Renderer;
        try {
            renderer = new Renderer({
                webgl: 2,
                alpha: true,
                premultipliedAlpha: true,
                antialias: false,
                // The lab is an exploratory surface; keep the shader bounded
                // on high-DPR displays so it cannot dominate the page budget.
                dpr: Math.min(window.devicePixelRatio || 1, 1.5),
            });
        } catch {
            return;
        }

        const gl = renderer.gl;
        gl.clearColor(0, 0, 0, 0);
        const canvas = gl.canvas;
        canvas.style.width = "100%";
        canvas.style.height = "100%";
        canvas.style.display = "block";
        canvas.setAttribute("aria-hidden", "true");
        container.appendChild(canvas);
        container.dataset.contextStatus = "webgl";

        const geometry = new Triangle(gl);
        const program = new Program(gl, {
            vertex,
            fragment,
            uniforms: {
                iTime: { value: 0 },
                iResolution: { value: new Float32Array([1, 1]) },
                uSpeed: { value: finiteClamp(speed, 0.24, 0, 4) },
                uScale: { value: finiteClamp(scale, 4, 0.5, 8) },
                uDetail: { value: finiteClamp(detail, 3, 1, 8) },
                uGlow: { value: finiteClamp(glow, 1.35, 0, 4) },
                uCoreSize: { value: finiteClamp(coreSize, 0.09, 0.001, 1) },
                uSwirl: { value: finiteClamp(swirl, 0.9, -4, 4) },
                uFold: { value: finiteClamp(fold, -0.2, -4, 4) },
                uBlackPoint: { value: finiteClamp(blackPoint, 0.08, 0, 1) },
                uBrightness: { value: finiteClamp(brightness, 1.05, 0, 4) },
                uColorMode: { value: colorModeToFloat(colorMode) },
                uGrain: { value: grain ? 1 : 0 },
                uGrainIntensity: { value: finiteClamp(grainIntensity, 0.02, 0, 0.2) },
                uOpacity: { value: finiteClamp(opacity, 0.58, 0, 1) },
                uMouse: { value: new Float32Array([0.5, 0.5]) },
                uMouseStrength: { value: finiteClamp(mouseStrength, 0.18, 0, 1) },
                uEnableMouse: { value: mouseInteraction },
                uColor1: { value: new Float32Array(hexToRgb(color1)) },
                uColor2: { value: new Float32Array(hexToRgb(color2)) },
                uColor3: { value: new Float32Array(hexToRgb(color3)) },
            },
        });

        const mesh = new Mesh(gl, { geometry, program });
        ctxMap.set(container, { renderer, program, mesh });
        let contextLost = false;

        const setSize = () => {
            if (contextLost) return;
            const rect = container.getBoundingClientRect();
            const width = Math.max(1, Math.floor(rect.width));
            const height = Math.max(1, Math.floor(rect.height));
            renderer.setSize(width, height);
            const resolution = program.uniforms.iResolution.value as Float32Array;
            resolution[0] = gl.drawingBufferWidth;
            resolution[1] = gl.drawingBufferHeight;
            renderer.render({ scene: mesh });
        };

        const resizeObserver = typeof ResizeObserver === "function" ? new ResizeObserver(setSize) : null;
        resizeObserver?.observe(container);
        const handleResize = () => setSize();
        window.addEventListener("resize", handleResize, { passive: true });
        setSize();

        const targetMouse = [0.5, 0.5];
        const currentMouse = [0.5, 0.5];
        const updateMouse = (clientX: number, clientY: number) => {
            if (!mouseInteraction) return;
            const rect = container.getBoundingClientRect();
            targetMouse[0] = (clientX - rect.left) / Math.max(rect.width, 1);
            targetMouse[1] = 1 - (clientY - rect.top) / Math.max(rect.height, 1);
        };
        const handlePointerMove = (event: PointerEvent) => updateMouse(event.clientX, event.clientY);
        const handlePointerLeave = () => {
            targetMouse[0] = 0.5;
            targetMouse[1] = 0.5;
        };
        window.addEventListener("pointermove", handlePointerMove, { passive: true });
        window.addEventListener("blur", handlePointerLeave, { passive: true });

        let raf = 0;
        let isVisible = true;
        let isPageVisible = !document.hidden;
        const motionQuery = motionPreference;
        const transparencyQuery = transparencyPreference;
        const forcedColorsQuery = forcedColorsPreference;
        const startedAt = performance.now();
        const frameInterval = 1000 / 45;
        let lastFrame = 0;

        const loop = (time: number) => {
            if (contextLost) {
                raf = 0;
                return;
            }
            if (lastFrame && time - lastFrame < frameInterval) {
                raf = requestAnimationFrame(loop);
                return;
            }
            lastFrame = time;
            program.uniforms.iTime.value = (time - startedAt) * 0.001;
            currentMouse[0] += 0.05 * (targetMouse[0] - currentMouse[0]);
            currentMouse[1] += 0.05 * (targetMouse[1] - currentMouse[1]);
            const mouse = program.uniforms.uMouse.value as Float32Array;
            mouse[0] = currentMouse[0];
            mouse[1] = currentMouse[1];
            renderer.render({ scene: mesh });
            raf = requestAnimationFrame(loop);
        };

        const tryStart = () => {
            if (!contextLost && isVisible && isPageVisible && !motionQuery.matches && !transparencyQuery.matches && !forcedColorsQuery.matches && raf === 0) raf = requestAnimationFrame(loop);
        };
        const tryStop = () => {
            if (raf !== 0) {
                cancelAnimationFrame(raf);
                raf = 0;
            }
        };

        const intersectionObserver =
            typeof IntersectionObserver === "function"
                ? new IntersectionObserver(([entry]) => {
                      isVisible = entry?.isIntersecting ?? true;
                      isVisible ? tryStart() : tryStop();
                  })
                : null;
        intersectionObserver?.observe(container);

        const onVisibility = () => {
            isPageVisible = !document.hidden;
            isPageVisible ? tryStart() : tryStop();
        };
        const onMotionPreference = () => {
            if (motionQuery.matches || transparencyQuery.matches || forcedColorsQuery.matches) {
                tryStop();
            } else {
                tryStart();
            }
        };
        const stopObservingContext = observeMoltenMetalContextLoss(canvas, (reason) => {
            contextLost = true;
            tryStop();
            canvas.hidden = true;
            container.dataset.contextStatus = reason === "lost" ? "lost-fallback" : "restored-fallback";
        });
        document.addEventListener("visibilitychange", onVisibility);
        motionQuery.addEventListener?.("change", onMotionPreference);
        transparencyQuery.addEventListener?.("change", onMotionPreference);
        forcedColorsQuery.addEventListener?.("change", onMotionPreference);
        tryStart();

        return () => {
            tryStop();
            resizeObserver?.disconnect();
            intersectionObserver?.disconnect();
            window.removeEventListener("resize", handleResize);
            document.removeEventListener("visibilitychange", onVisibility);
            motionQuery.removeEventListener?.("change", onMotionPreference);
            transparencyQuery.removeEventListener?.("change", onMotionPreference);
            forcedColorsQuery.removeEventListener?.("change", onMotionPreference);
            stopObservingContext();
            window.removeEventListener("pointermove", handlePointerMove);
            window.removeEventListener("blur", handlePointerLeave);
            ctxMap.delete(container);
            program.remove();
            try {
                container.removeChild(canvas);
            } catch {
                // The host may already have been removed during route changes.
            }
            const loseContext = gl.getExtension("WEBGL_lose_context") as { loseContext?: () => void } | null;
            loseContext?.loseContext?.();
        };
    }, []);

    useEffect(() => {
        const container = containerRef.current;
        if (!container) return;
        const context = ctxMap.get(container);
        if (!context) return;
        const uniforms = context.program.uniforms;

        uniforms.uSpeed.value = finiteClamp(speed, 0.24, 0, 4);
        uniforms.uScale.value = finiteClamp(scale, 4, 0.5, 8);
        uniforms.uDetail.value = finiteClamp(detail, 3, 1, 8);
        uniforms.uGlow.value = finiteClamp(glow, 1.35, 0, 4);
        uniforms.uCoreSize.value = finiteClamp(coreSize, 0.09, 0.001, 1);
        uniforms.uSwirl.value = finiteClamp(swirl, 0.9, -4, 4);
        uniforms.uFold.value = finiteClamp(fold, -0.2, -4, 4);
        uniforms.uBlackPoint.value = finiteClamp(blackPoint, 0.08, 0, 1);
        uniforms.uBrightness.value = finiteClamp(brightness, 1.05, 0, 4);
        uniforms.uColorMode.value = colorModeToFloat(colorMode);
        uniforms.uGrain.value = grain ? 1 : 0;
        uniforms.uGrainIntensity.value = finiteClamp(grainIntensity, 0.02, 0, 0.2);
        uniforms.uOpacity.value = finiteClamp(opacity, 0.58, 0, 1);
        uniforms.uMouseStrength.value = finiteClamp(mouseStrength, 0.18, 0, 1);
        uniforms.uEnableMouse.value = mouseInteraction;

        const colorValues = [hexToRgb(color1), hexToRgb(color2), hexToRgb(color3)];
        [uniforms.uColor1.value, uniforms.uColor2.value, uniforms.uColor3.value].forEach((target, index) => {
            target[0] = colorValues[index][0];
            target[1] = colorValues[index][1];
            target[2] = colorValues[index][2];
        });
    }, [color1, color2, color3, speed, scale, detail, glow, coreSize, swirl, fold, blackPoint, brightness, colorMode, grain, grainIntensity, mouseInteraction, mouseStrength, opacity]);

    return <div ref={containerRef} className={[styles.container, className].filter(Boolean).join(" ")} data-ocean-background="molten-metal" aria-hidden="true" />;
}

export default MoltenMetal;
