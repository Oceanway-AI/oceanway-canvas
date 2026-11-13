import { lazy, Suspense } from "react";
import { useAuthDialogStore } from "@/stores/use-auth-dialog-store";
import { useUserStore } from "@/stores/use-user-store";

const PlatformLoginPanel = lazy(() => import("./platform-login-panel"));

export function PlatformLoginDialogHost() {
    const { open, closeDialog } = useAuthDialogStore();
    const user = useUserStore((state) => state.user);
    if (!open || user) return null;
    return (
        <Suspense fallback={null}>
            <PlatformLoginPanel next={window.location.pathname + window.location.search + window.location.hash} onDismiss={closeDialog} />
        </Suspense>
    );
}
