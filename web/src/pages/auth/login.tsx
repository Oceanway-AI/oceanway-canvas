import { Navigate, useLocation, useNavigate, useSearchParams } from "react-router";
import PlatformLoginPanel from "@/components/auth/platform-login-panel";
import { platformLoginNext } from "@/lib/platform-login";
import { useUserStore } from "@/stores/use-user-store";

export default function LoginPage() {
    const location = useLocation();
    const [params] = useSearchParams();
    const navigate = useNavigate();
    const user = useUserStore((state) => state.user);
    const next = platformLoginNext(params.get("next"));
    if (user) return <Navigate to={next} replace />;
    return <PlatformLoginPanel next={next} recovery={location.pathname.replace(/\/+$/, "").toLowerCase() === "/forgot-password"} onDismiss={() => navigate("/", { replace: true })} />;
}
