import React, { useLayoutEffect, useSyncExternalStore } from "react";
import { createRoot } from "react-dom/client";
import "antd/dist/reset.css";
import "./styles/globals.css";
import "./styles/shared/model-picker.css";
import "./styles/shared/overlays.css";
import "./styles/shared/scrollbars.css";
import "./styles/oceanway-palette.css";
import "./styles/oceanway.css";
import { RouterProvider } from "react-router";

import { PlatformLoginDialogHost } from "@/components/auth/platform-login-dialog-host";
import { AppProviders } from "@/components/layout/app-providers";
import { router } from "@/router";

const subscribeToRoute = (callback: () => void) => router.subscribe(callback);
const currentPath = () => router.state.location.pathname;

function Application() {
    const pathname = useSyncExternalStore(subscribeToRoute, currentPath);
    const oceanway = !/^\/admin(?:\/|$)/.test(pathname);
    useLayoutEffect(() => {
        document.documentElement.toggleAttribute("data-oceanway", oceanway);
    }, [oceanway]);
    return (
        <AppProviders oceanway={oceanway}>
            <RouterProvider router={router} />
            <PlatformLoginDialogHost />
        </AppProviders>
    );
}

document.body.style.fontFamily = '"SF Pro Display","SF Pro Text","PingFang SC","Microsoft YaHei","Helvetica Neue",sans-serif';

createRoot(document.getElementById("root")!).render(
    <React.StrictMode>
        <Application />
    </React.StrictMode>,
);
