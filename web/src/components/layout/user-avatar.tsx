import { OceanAvatar } from "@/vendor/oceanway-account-v1/account";
import { cn } from "@/lib/utils";
import type { LocalUser } from "@/stores/use-user-store";

export function UserAvatar({ user, className }: { user: LocalUser; className?: string }) {
    return (
        <span className={cn("grid shrink-0 place-items-center overflow-hidden", className)}>
            <OceanAvatar displayName={user.displayName || user.username} avatarUrl={user.avatarUrl} />
        </span>
    );
}
