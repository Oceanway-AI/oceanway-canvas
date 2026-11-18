import type { DramaCast, DramaCastKind, DramaStatus } from "@/services/api/drama";

// 短剧文案与数字格式：制作台和精修画布共用，避免两边各写一份。

export const DRAMA_CAST_KIND_LABEL: Record<DramaCastKind | "image", string> = { character: "角色", prop: "道具", scene: "场景", image: "图片" };

export const DRAMA_STATUS_LABEL: Record<DramaStatus, string> = { draft: "未开始", running: "制作中", paused: "已暂停", completed: "已完成" };

export function dramaCastDisplayName(cast: Pick<DramaCast, "name" | "lookName">) {
    return cast.lookName ? `${cast.name} · ${cast.lookName}` : cast.name;
}
