import { useCallback } from "react";
import { useNavigate } from "react-router";
import { useAuthDialogStore } from "@/stores/use-auth-dialog-store";

export function useAuthEntry() {
    const navigate = useNavigate();
    const openAuth = useAuthDialogStore((state) => state.openDialog);
    const showLoginAfterSignOut = useCallback(() => navigate("/login", { replace: true }), [navigate]);
    return { openAuth, showLoginAfterSignOut };
}
