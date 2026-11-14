import { Plus, X } from "lucide-react";
import type { FrameRole } from "@/lib/media-reference-slots";
import "./reference-slot.css";

export function ReferenceSlot({ role, label = "参考内容", src, disabled, onPick, onRemove, title }: { role?: FrameRole; label?: string; src?: string; disabled?: boolean; onPick: () => void; onRemove?: () => void; title?: string }) {
    const name = role === "first_frame" ? "首帧" : role === "last_frame" ? "尾帧" : label;
    const action = (onRemove ? "替换" : "添加") + name;
    return (
        <div className="studio-reference-slot" data-empty={!onRemove}>
            <button type="button" disabled={disabled} onClick={onPick} title={title || action} aria-label={title || action}>
                {src ? <img src={src} alt="" /> : <Plus size={17} />}
                <span>{name}</span>
            </button>
            {onRemove && (
                <button type="button" className="studio-slot-remove" aria-label={"移除" + name} disabled={disabled} onClick={onRemove}>
                    <X size={12} />
                </button>
            )}
        </div>
    );
}
