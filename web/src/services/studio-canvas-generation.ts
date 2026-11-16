import type { Dispatch, SetStateAction } from "react";
import type { CanvasNodeData } from "@/types/canvas";
import type { GenerationTask } from "@/services/api/task-center";
import { getActiveUserScope } from "@/lib/user-scope";
import { projectStudioCanvasOutput, studioCanvasOutputs } from "@/lib/canvas/studio-generation-outputs";
import { persistCanvasGenerationEffect, prepareCanvasMediaNodeAsset } from "@/services/canvas-generation-consumer";

type View = { nodesRef: { current: CanvasNodeData[] }; setNodes: Dispatch<SetStateAction<CanvasNodeData[]>> };
const observations = new Map<string, Promise<void>>();

export function adoptStudioCanvasGeneration(projectId: string, rootId: string, task: GenerationTask, view: View, signal?: AbortSignal) {
    const scope = getActiveUserScope();
    const key = JSON.stringify([scope, projectId, rootId, task.id]);
    const check = () => {
        signal?.throwIfAborted();
        if (scope !== getActiveUserScope()) throw new DOMException("Account changed", "AbortError");
    };
    const apply = async () => {
        const failed: unknown[] = [];
        for (const slot of studioCanvasOutputs(task)) {
            try {
                check();
                const before = view.nodesRef.current;
                const change = projectStudioCanvasOutput(before, rootId, task, slot);
                if (!change.target) continue;
                const target = slot.resourceId ? await prepareCanvasMediaNodeAsset(projectId, change.target, signal) : change.target;
                check();
                const project = (nodes: CanvasNodeData[]) => {
                    const next = projectStudioCanvasOutput(nodes, rootId, task, slot);
                    return target.metadata?.assetId && next.target ? next.nodes.map((n) => (n.id === target.id ? { ...n, metadata: { ...n.metadata, assetId: target.metadata?.assetId } } : n)) : next.nodes;
                };
                const previousNodes = view.nodesRef.current;
                const nodes = project(previousNodes);
                if (nodes === previousNodes) continue;
                await persistCanvasGenerationEffect({ projectId, previousNodes, nodes, effectKey: change.effectKey, signal });
                check();
                view.setNodes(project);
            } catch (error) {
                check();
                failed.push(error);
            }
        }
        if (failed.length) throw failed[0];
    };
    const previous = observations.get(key);
    const pending = (previous ? previous.catch(() => undefined).then(apply) : apply()).finally(() => {
        if (observations.get(key) === pending) observations.delete(key);
    });
    observations.set(key, pending);
    return pending;
}
