import { oceanAirPalette } from "@/lib/ocean-air-theme";

export type CanvasColorTheme = "light" | "dark";
export type CanvasBackgroundMode = "dots" | "lines" | "blank";
export const DEFAULT_CANVAS_COLOR_THEME: CanvasColorTheme = "light";
export const CANVAS_CONNECTION_STROKE_WIDTH = 1.4;

export const canvasThemes = {
    light: {
        connection: {
            stroke: "#8da7c2",
            beam: oceanAirPalette.light.primary,
        },
        canvas: {
            background: oceanAirPalette.light.background,
            dot: "rgba(46,61,78,.23)",
            line: "rgba(46,61,78,.23)",
            selectionFill: "rgba(47,127,211,.10)",
        },
        node: {
            label: oceanAirPalette.light.textMuted,
            agentUserMessage: "#edf6ff",
            fill: oceanAirPalette.light.surface,
            panel: oceanAirPalette.light.surface,
            stroke: oceanAirPalette.light.border,
            edge: "rgba(15,23,42,.16)",
            shadow: "0 6px 18px rgba(15,23,42,.08)",
            hoverShadow: "0 10px 24px rgba(15,23,42,.12)",
            activeStroke: oceanAirPalette.light.primary,
            placeholder: "#9ca3af",
            text: oceanAirPalette.light.text,
            muted: oceanAirPalette.light.textMuted,
            faint: "#9ca3af",
        },
        frame: {
            fill: "rgba(17,24,39,.025)",
            stroke: "rgba(17,24,39,.18)",
            activeFill: "rgba(47,127,211,.05)",
            activeStroke: oceanAirPalette.light.primary,
            preview: "rgba(255,255,255,.82)",
        },
        toolbar: {
            panel: "rgba(248,250,252,.94)",
            border: oceanAirPalette.light.border,
            item: oceanAirPalette.light.textMuted,
            itemHover: oceanAirPalette.light.surfaceSoft,
            activeBg: oceanAirPalette.light.surfaceStrong,
            activeText: oceanAirPalette.light.primary,
        },
        spatial: {
            surface: "rgba(255,255,255,.72)",
            elevated: "rgba(255,255,255,.94)",
            dropzone: "rgba(248,250,252,.78)",
            glow: "rgba(47,127,211,.14)",
            glowStrong: "rgba(47,127,211,.42)",
            shadow: "rgba(15,23,42,.18)",
        },
        // 时间线/字幕组件语义色：轨道底、按类型区分的片段、标尺与播放头、字幕列表条目态。
        timeline: {
            trackFill: "#f6f8fb",
            trackBorder: "rgba(17,24,39,.08)",
            clipVideo: "rgba(79,110,232,.16)",
            clipAudio: "rgba(16,185,129,.14)",
            clipSubtitle: "rgba(245,158,11,.16)",
            clipSelectedBorder: oceanAirPalette.light.primary,
            handle: "rgba(255,255,255,.72)",
            rulerTick: "rgba(17,24,39,.28)",
            rulerLabel: "#6b7280",
            playhead: oceanAirPalette.light.primary,
            entryActive: "rgba(17,17,17,.10)",
            entryHover: "rgba(17,24,39,.05)",
        },
        accent: {
            primary: oceanAirPalette.light.primary,
            primaryRgb: "47 127 211",
            primarySoft: "rgba(47,127,211,.10)",
            onPrimary: "#ffffff",
            danger: "#f87171",
        },
    },
    dark: {
        connection: {
            stroke: "#627c98",
            beam: oceanAirPalette.dark.primary,
        },
        canvas: {
            background: oceanAirPalette.dark.background,
            dot: "rgba(232,241,250,.24)",
            line: "rgba(232,241,250,.24)",
            selectionFill: "rgba(10,132,255,.14)",
        },
        node: {
            label: oceanAirPalette.dark.textMuted,
            agentUserMessage: "#182b40",
            fill: oceanAirPalette.dark.surface,
            panel: oceanAirPalette.dark.surface,
            stroke: oceanAirPalette.dark.border,
            edge: "rgba(255,255,255,.18)",
            shadow: "0 8px 24px rgba(0,0,0,.34)",
            hoverShadow: "0 12px 30px rgba(0,0,0,.46)",
            activeStroke: oceanAirPalette.dark.primary,
            placeholder: "#737373",
            text: oceanAirPalette.dark.text,
            muted: oceanAirPalette.dark.textMuted,
            faint: "#666666",
        },
        frame: {
            fill: "rgba(255,255,255,.025)",
            stroke: "rgba(190,198,210,.15)",
            activeFill: "rgba(10,132,255,.08)",
            activeStroke: oceanAirPalette.dark.primary,
            preview: "rgba(26,32,40,.94)",
        },
        toolbar: {
            panel: "rgba(26,32,40,.97)",
            border: oceanAirPalette.dark.border,
            item: oceanAirPalette.dark.textMuted,
            itemHover: oceanAirPalette.dark.surfaceSoft,
            activeBg: oceanAirPalette.dark.surfaceStrong,
            activeText: oceanAirPalette.dark.primary,
        },
        spatial: {
            surface: "rgba(26,32,40,.82)",
            elevated: "rgba(26,32,40,.97)",
            dropzone: "rgba(17,22,29,.9)",
            glow: "rgba(10,132,255,.12)",
            glowStrong: "rgba(10,132,255,.36)",
            shadow: "rgba(0,0,0,.6)",
        },
        timeline: {
            trackFill: oceanAirPalette.dark.background,
            trackBorder: "rgba(255,255,255,.08)",
            clipVideo: "rgba(96,126,234,.22)",
            clipAudio: "rgba(16,185,129,.20)",
            clipSubtitle: "rgba(245,158,11,.20)",
            clipSelectedBorder: oceanAirPalette.dark.primary,
            handle: "rgba(255,255,255,.5)",
            rulerTick: "rgba(255,255,255,.26)",
            rulerLabel: "#a3a3a3",
            playhead: oceanAirPalette.dark.primary,
            entryActive: "rgba(255,255,255,.10)",
            entryHover: "rgba(255,255,255,.06)",
        },
        accent: {
            primary: oceanAirPalette.dark.primary,
            primaryRgb: "10 132 255",
            primarySoft: "rgba(10,132,255,.11)",
            onPrimary: oceanAirPalette.dark.onPrimary,
            danger: "#fb7185",
        },
    },
} as const;

export type CanvasTheme = (typeof canvasThemes)[CanvasColorTheme];
