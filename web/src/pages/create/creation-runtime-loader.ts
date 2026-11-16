export type CreationRuntime = typeof import("./creation-runtime");

let runtime: Promise<CreationRuntime> | undefined;

export function loadCreationRuntime(): Promise<CreationRuntime> {
    runtime ??= import("./creation-runtime").catch((error) => {
        runtime = undefined;
        throw error;
    });
    return runtime;
}
