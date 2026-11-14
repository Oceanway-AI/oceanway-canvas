import type { NativeMediaVariant } from "./platform-native-media";

export type FrameRole = "first_frame" | "last_frame";

export function publishedFrameRoles(variant?: NativeMediaVariant): FrameRole[] {
    const refs = variant?.native_request_variant.native_reference_inputs?.filter((ref) => ref.max_items !== 0) || [];
    if (refs.some((ref) => ref.role !== "first_frame" && ref.role !== "last_frame")) return [];
    return (["first_frame", "last_frame"] as const).filter((role) => refs.some((ref) => ref.role === role));
}

// Honor explicit slots first, then fill the remaining slots in attachment order for existing drafts.
export function assignFrameSlots<T extends { id: string }>(roles: FrameRole[], images: T[], explicit: (image: T) => string | undefined) {
    const slots = roles.map((role) => ({ role, attachment: images.find((image) => explicit(image) === role) }));
    const used = new Set(slots.map((slot) => slot.attachment?.id));
    const remaining = images.filter((image) => !used.has(image.id));
    for (const slot of slots) slot.attachment ??= remaining.shift();
    return slots;
}
