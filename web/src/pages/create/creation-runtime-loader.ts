export type CreationRuntime = typeof import("./creation-runtime");

export const loadCreationRuntime = (): Promise<CreationRuntime> => import("./creation-runtime");
