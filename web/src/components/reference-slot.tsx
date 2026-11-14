import { Plus, X } from "lucide-react";
import { useEffect, useRef, useState, type ReactNode } from "react";
import type { FrameRole } from "@/lib/media-reference-slots";
import "./reference-slot.css";

export function ReferenceStack({ count, pinned = false, children }: { count: number; pinned?: boolean; children: (open: boolean) => ReactNode }) {
    const root = useRef<HTMLDivElement>(null);
    const [hovered, setHovered] = useState(false);
    const [focused, setFocused] = useState(false);
    const [tapped, setTapped] = useState(false);
    const open = pinned || hovered || focused || tapped;

    useEffect(() => {
        if (!tapped) return;
        const dismiss = (event: globalThis.PointerEvent) => {
            if (!root.current?.contains(event.target as Node)) setTapped(false);
        };
        document.addEventListener("pointerdown", dismiss, true);
        return () => document.removeEventListener("pointerdown", dismiss, true);
    }, [tapped]);

    return (
        <div
            ref={root}
            className="studio-reference-stack"
            data-open={open}
            onPointerEnter={(event) => {
                if (event.pointerType === "mouse" || event.pointerType === "pen") setHovered(true);
            }}
            onPointerLeave={() => setHovered(false)}
            onFocusCapture={(event) => {
                if (event.target.matches(":focus-visible")) setFocused(true);
            }}
            onBlurCapture={(event) => {
                if (!event.currentTarget.contains(event.relatedTarget)) {
                    setFocused(false);
                    setTapped(false);
                }
            }}
            onKeyDown={(event) => {
                if (event.key !== "Escape" || !open) return;
                event.stopPropagation();
                root.current?.querySelector<HTMLButtonElement>(".studio-stack-trigger")?.focus();
                setHovered(false);
                setFocused(false);
                setTapped(false);
            }}
        >
            <button type="button" className="studio-stack-trigger" tabIndex={open ? -1 : 0} aria-label={`展开 ${count} 个参考素材`} aria-expanded={open} onClick={() => setTapped(true)} />
            <div className="studio-stack-items" inert={!open}>
                {children(open)}
            </div>
        </div>
    );
}

export function FrameReferenceSlots({ filled, children }: { filled: number; children: ReactNode }) {
    const slots = <div className="studio-reference-slots studio-frame-slots">{children}</div>;
    return filled > 1 ? <ReferenceStack count={filled}>{() => slots}</ReferenceStack> : slots;
}

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
