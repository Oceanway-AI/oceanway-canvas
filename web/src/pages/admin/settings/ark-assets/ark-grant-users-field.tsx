import { Select } from "@/components/ui/base/select";
import type { ArkUserSummary } from "@/services/api/ark-portraits";
import { arkUserLabel } from "./ark-admin-shared";

export function ArkGrantUsersField({ value, onChange, known, disabled }: { value: string[]; onChange: (ids: string[]) => void; known?: ArkUserSummary[]; disabled?: boolean }) {
    return (
        <Select
            mode="tags"
            allowClear
            disabled={disabled}
            options={(known || []).map((user) => ({ value: user.id, label: arkUserLabel(user) }))}
            value={value}
            onChange={(next) => onChange((next as string[]).filter((id) => /^[1-9]\d*$/.test(id)))}
            placeholder="输入 Platform 用户 ID"
            ariaLabel="授权用户"
            className="w-full"
        />
    );
}
