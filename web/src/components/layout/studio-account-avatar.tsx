import { OceanAvatar } from "@/vendor/oceanway-account-v1/account";
import { OceanWorkspaceAvatar } from "@/vendor/oceanway-context-switcher-v1/context-switcher";

export function StudioTeamAvatar({ name }: { name: string }) {
    return <span className="ow-studio-team-avatar">{Array.from(name.trim())[0] || "团"}</span>;
}

export function StudioAccountAvatar({ name, avatarUrl, teamName, size = 28 }: { name: string; avatarUrl?: string; teamName?: string; size?: number }) {
    return <OceanWorkspaceAvatar size={size} avatar={<OceanAvatar displayName={name} avatarUrl={avatarUrl} size={size} />} teamAvatar={teamName ? <StudioTeamAvatar name={teamName} /> : undefined} />;
}
