import { create } from "zustand";
import type { PickerPreviewRow } from "@oceanway/model-picker/preview-cache";

export type ModelCatalogStatus = "idle" | "loading" | "ready" | "error";

// A server-confirmed catalog is session memory, never a persisted authority.
export const useModelCatalogStore = create<{
    status: ModelCatalogStatus;
    error: string | null;
    preview: PickerPreviewRow[];
}>(() => ({ status: "idle", error: null, preview: [] }));
