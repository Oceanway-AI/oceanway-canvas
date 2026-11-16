import type { GenerationTask } from "@/services/api/task-center";

type Flight = { controller: AbortController; promise: Promise<GenerationTask>; readers: number };

export function createStudioGenerationReads(scope: () => string, fetchTask: (id: string, signal: AbortSignal) => Promise<GenerationTask>, now = Date.now) {
    const snapshots = new Map<string, { task: GenerationTask; due: number }>();
    const requests = new Map<string, Flight>();
    const key = (id: string) => JSON.stringify([scope(), id]);
    const abortError = () => new DOMException("Observation stopped", "AbortError");
    const remember = (task: GenerationTask) => {
        if (!task.platformGenerationRequest) return task;
        const id = key(task.id);
        const previous = snapshots.get(id);
        if (previous && (previous.task.platformRevision ?? 0) > (task.platformRevision ?? 0)) return previous.task;
        const terminal = task.status === "failed" || task.status === "cancelled" || task.status === "succeeded";
        snapshots.delete(id);
        snapshots.set(id, { task, due: terminal ? Infinity : now() + Math.max(1, task.retryAfterMs || 2000) });
        if (snapshots.size > 256) snapshots.delete(snapshots.keys().next().value!);
        return task;
    };
    const read = (id: string, signal?: AbortSignal): Promise<GenerationTask> => {
        const identity = key(id);
        if (signal?.aborted) return Promise.reject(abortError());
        const cached = snapshots.get(identity);
        if (cached && now() < cached.due) return Promise.resolve(cached.task);
        let flight = requests.get(identity);
        if (!flight || flight.controller.signal.aborted) {
            const controller = new AbortController();
            const promise = fetchTask(id, controller.signal)
                .then((task) => {
                    if (controller.signal.aborted || key(id) !== identity) throw abortError();
                    return remember(task);
                })
                .finally(() => {
                    if (requests.get(identity)?.controller === controller) requests.delete(identity);
                });
            flight = { controller, promise, readers: 0 };
            requests.set(identity, flight);
        }
        const active = flight;
        active.readers++;
        return new Promise<GenerationTask>((resolve, reject) => {
            let finished = false;
            const detach = () => {
                if (finished) return false;
                finished = true;
                signal?.removeEventListener("abort", abort);
                active.readers--;
                if (!active.readers && requests.get(identity) === active) active.controller.abort();
                return true;
            };
            const abort = () => {
                if (detach()) reject(abortError());
            };
            signal?.addEventListener("abort", abort, { once: true });
            active.promise.then(
                (task) => {
                    if (detach()) resolve(task);
                },
                (error) => {
                    if (detach()) reject(error);
                },
            );
        });
    };
    return { read, remember };
}
