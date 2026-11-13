import { Link, useLocation, type LinkProps } from "react-router";
import { platformLoginPage } from "@/lib/platform-login";
import { useAuthDialogStore } from "@/stores/use-auth-dialog-store";

export function AuthLoginLink({ onClick, ...props }: Omit<LinkProps, "to">) {
    const location = useLocation();
    const openDialog = useAuthDialogStore((state) => state.openDialog);
    return (
        <Link
            {...props}
            to={platformLoginPage(location.pathname + location.search + location.hash)}
            onClick={(event) => {
                onClick?.(event);
                if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || (props.target && props.target !== "_self")) return;
                event.preventDefault();
                openDialog();
            }}
        />
    );
}
