import { lazy, Suspense } from "react";
import { oceanAirPalette } from "@/lib/ocean-air-theme";
import { useThemeStore } from "@/stores/use-theme-store";

const MoltenMetal = lazy(() => import("./backgrounds/molten-metal").then((module) => ({ default: module.MoltenMetal })));

export function OceanwayBackground() {
    const dark = useThemeStore((state) => state.theme === "dark");
    const palette = dark ? oceanAirPalette.dark : oceanAirPalette.light;
    return (
        <div className="oceanway-background" aria-hidden="true">
            <Suspense fallback={null}>
                <MoltenMetal
                    color1={palette.primary}
                    color2={palette.sky}
                    color3={palette.surfaceStrong}
                    speed={0.3}
                    scale={3.4}
                    detail={4}
                    glow={1.85}
                    coreSize={0.13}
                    swirl={1}
                    fold={-0.2}
                    blackPoint={0.015}
                    brightness={1.3}
                    colorMode="frost"
                    grain={false}
                    mouseInteraction
                    mouseStrength={0.2}
                    opacity={0.86}
                />
            </Suspense>
        </div>
    );
}
