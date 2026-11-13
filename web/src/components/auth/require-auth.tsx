import { lazy, Suspense, type ReactNode } from "react";
import { useLocation, useNavigate } from "react-router";
import { FullScreenLoader } from "@/components/ui/aceternity/full-screen-loader";
import { useUserStore } from "@/stores/use-user-store";

const PlatformLoginPanel = lazy(() => import("./platform-login-panel"));

export function RequireAuth({ children, allowAnonymous = false }: { children: ReactNode; allowAnonymous?: boolean }) {
    const location = useLocation();
    const navigate = useNavigate();
    const hydrated = useUserStore((state) => state.hydrated);
    const user = useUserStore((state) => state.user);
    if (!hydrated) return <FullScreenLoader />;
    if (!user && !allowAnonymous) {
        return (
            <Suspense fallback={<FullScreenLoader />}>
                <PlatformLoginPanel next={location.pathname + location.search + location.hash} onDismiss={() => navigate("/", { replace: true })} />
            </Suspense>
        );
    }
    return children;
}
