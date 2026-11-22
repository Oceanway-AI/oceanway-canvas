import type { NativeMediaVariant } from "./platform-native-media";

export type FrameRole = "first_frame" | "last_frame";

export function publishedFrameRoles(variant?: NativeMediaVariant): FrameRole[] {
    const refs = variant?.native_request_variant.native_reference_inputs?.filter((ref) => ref.max_items !== 0) || [];
    return (["first_frame", "last_frame"] as const).filter((role) => refs.some((ref) => ref.role === role));
}

// Only frame-only contracts can interpret an unlabelled image as an ordered frame.
export function assignFrameSlots<T>(variant: NativeMediaVariant | undefined, images: T[], explicit: (image: T) => string | undefined) {
    const slots = publishedFrameRoles(variant).map((role) => ({ role, attachment: images.find((image) => explicit(image) === role) }));
    if (variant?.native_request_variant.native_reference_inputs?.some((ref) => ref.role === "reference_image" && ref.max_items !== 0)) return slots;
    const remaining = images.filter((image) => !explicit(image));
    for (const slot of slots) slot.attachment ??= remaining.shift();
    return slots;
}

export function requestFrameImages<T extends { role?: string }>(variant: NativeMediaVariant | undefined, images: T[], explicit: (image: T) => string | undefined = (image) => image.role): T[] {
    const slots = assignFrameSlots(variant, images, explicit);
    const assigned = new Set(slots.map((slot) => slot.attachment));
    return [...slots.flatMap(({ role, attachment }) => (attachment ? [{ ...attachment, role }] : [])), ...images.filter((image) => !assigned.has(image)).map((image) => ({ ...image, role: explicit(image) }))];
}
