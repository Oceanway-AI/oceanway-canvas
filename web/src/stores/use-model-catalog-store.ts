import { create } from "zustand";

export type ModelCatalogStatus = "idle" | "loading" | "ready" | "error";

// A server-confirmed catalog is session memory, never a persisted authority.
export const useModelCatalogStore = create<{
    status: ModelCatalogStatus;
    error: string | null;
}>(() => ({ status: "idle", error: null }));
