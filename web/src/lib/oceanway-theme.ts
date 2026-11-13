import type { ThemeConfig } from "antd";
import { getAntThemeConfig } from "@/lib/app-theme";
import { oceanAirPalette } from "@/lib/ocean-air-theme";

export const OCEANWAY_NAME = "Oceanway Studio";
export const OCEANWAY_FONT = '-apple-system, BlinkMacSystemFont, "SF Pro Text", "PingFang SC", "Microsoft YaHei", "Helvetica Neue", Arial, sans-serif';

/** Studio presentation on Canvas's existing controls; admin retains its own theme. */
export function getOceanwayAntThemeConfig(dark: boolean, skin: unknown): ThemeConfig {
    const base = getAntThemeConfig(dark, skin);
    const palette = dark ? oceanAirPalette.dark : oceanAirPalette.light;
    return {
        ...base,
        cssVar: { key: `oceanway-${dark ? "dark" : "light"}` },
        token: {
            ...base.token,
            fontFamily: OCEANWAY_FONT,
            colorPrimary: palette.primary,
            colorPrimaryHover: palette.primaryHover,
            colorPrimaryActive: palette.actionFill,
            colorInfo: palette.primary,
            colorText: palette.text,
            colorTextSecondary: palette.textMuted,
            colorTextTertiary: palette.textQuiet,
            colorBgBase: palette.background,
            colorBgContainer: palette.surface,
            colorBgElevated: palette.surface,
            colorBorder: palette.border,
            colorBorderSecondary: palette.border,
            colorLink: palette.text,
            colorLinkHover: palette.text,
            colorLinkActive: palette.text,
            colorSuccess: palette.success,
            colorWarning: palette.warning,
            colorError: palette.danger,
            borderRadius: 14,
            borderRadiusLG: 24,
            borderRadiusSM: 10,
            controlHeight: 40,
            controlHeightSM: 32,
            controlOutlineWidth: 2,
            controlOutline: palette.spotlight,
        },
        components: {
            ...base.components,
            Button: { ...base.components?.Button, borderRadius: 14, borderRadiusSM: 10, borderRadiusLG: 16, colorPrimary: palette.actionFill, colorPrimaryHover: palette.actionHover, colorPrimaryActive: palette.actionActive, primaryColor: palette.onPrimary },
            Input: { ...base.components?.Input, borderRadius: 14, activeBg: palette.surface, hoverBg: palette.surface, activeBorderColor: palette.primary, hoverBorderColor: palette.borderStrong },
            InputNumber: { ...base.components?.InputNumber, borderRadius: 14, activeBg: palette.surface, hoverBg: palette.surface, activeBorderColor: palette.primary, hoverBorderColor: palette.borderStrong },
            Select: { ...base.components?.Select, borderRadius: 14, colorBgElevated: palette.surface, optionSelectedColor: palette.text, optionSelectedBg: palette.surfaceStrong },
            Modal: { ...base.components?.Modal, borderRadiusLG: 28, contentBg: palette.surface, headerBg: palette.surface },
            Drawer: { ...base.components?.Drawer, colorBgElevated: palette.surface },
            Dropdown: { ...base.components?.Dropdown, colorBgElevated: palette.surface, borderRadiusLG: 16 },
            Popover: { ...base.components?.Popover, colorBgElevated: palette.surface, borderRadiusLG: 20 },
            Switch: { ...base.components?.Switch, colorPrimary: palette.success, colorPrimaryHover: palette.success },
        },
    };
}
