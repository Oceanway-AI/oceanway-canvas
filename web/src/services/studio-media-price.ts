import { useEffect, useRef, useState } from "react";
import { imageRequestConfig, studioMediaGenerationVariant } from "@/lib/platform-native-media";
import { resourceIdFromStorageKey, resourceStorageKey } from "@/services/api/resources";
import { backendProviderConfig } from "@/services/api/generation-provider-config";
import type { CreateTaskInput } from "@/services/api/task-center";
import { http } from "@/services/api/request";
import { validPlatformPoints } from "@/services/platform-budget-confirmation";
import { modelOptionName, resolveModelChannel, type AiConfig } from "@/stores/use-config-store";
import { useUserStore } from "@/stores/use-user-store";

type EstimateReference = { kind: string; storageKey?: string; role?: string };
type Estimate = { state: "available" | "unavailable"; points?: string; priceVersion: string; variantId: string };
type PriceDisplay = { key: string; label: string };

export function studioMediaPriceRequest(config: AiConfig, mode: string, prompt: string, references: EstimateReference[], owner: string, projectId?: string): CreateTaskInput | null {
    if (!owner || !prompt.trim() || (mode !== "image" && mode !== "video")) return null;
    const selected = resolveModelChannel(config, config.model).modelDetails?.find((item) => item.model === modelOptionName(config.model))?.platform;
    if (!selected?.price.version) return null;
    const refs = { image: [] as EstimateReference[], video: [] as EstimateReference[], audio: [] as EstimateReference[] };
    for (const ref of references) {
        const kind = ref.kind === "character" ? "image" : ref.kind;
        if (kind !== "image" && kind !== "video" && kind !== "audio") continue;
        const id = resourceIdFromStorageKey(ref.storageKey);
        if (!id) return null;
        refs[kind].push({ kind, storageKey: resourceStorageKey(id), role: ref.role });
    }
    if (refs.image.some((r) => r.role === "first_frame" || r.role === "last_frame")) {
        refs.image.sort((a, b) => Number(a.role === "last_frame") - Number(b.role === "last_frame"));
    }
    try {
        if (mode === "image") config = imageRequestConfig(config, refs.image.length);
        const variantId = studioMediaGenerationVariant(config);
        if (!variantId) return null;
        const parameters = backendProviderConfig(config, mode);
        if (!("channelId" in parameters) || parameters.channelId !== "oceanway-platform") return null;
        const pins = (entries: EstimateReference[]) => entries.map(({ storageKey, role }) => ({ storageKey, role }));
        return {
            projectId,
            type: "canvas_" + mode,
            model: config.model,
            prompt,
            platform: {
                mediaGenerationV1: true,
                expectedUserId: owner,
                variantId,
                expectedPriceVersion: selected.price.version,
                ...(selected.capability_revision ? { expectedCapabilityRevision: selected.capability_revision, expectedConnectionRevision: selected.connection_revision } : {}),
            },
            input: { mode, config: parameters, referenceImages: pins(refs.image), referenceVideos: pins(refs.video), referenceAudios: pins(refs.audio) },
        };
    } catch {
        // Incomplete native parameter edits can still be corrected in the composer.
        return null;
    }
}

export function useStudioMediaPrice(config: AiConfig, mode: string, prompt: string, references: EstimateReference[], projectId?: string) {
    const owner = useUserStore((state) => state.user?.id || "");
    const input = studioMediaPriceRequest(config, mode, prompt, references, owner, projectId);
    const key = input ? JSON.stringify(input) : "";
    const recent = useRef(new Map<string, { label: string; expires: number }>());
    const [display, setDisplay] = useState<PriceDisplay>();
    useEffect(() => {
        if (!key) return;
        const cached = recent.current.get(key);
        if (cached && cached.expires > Date.now()) {
            setDisplay({ key, label: cached.label });
            return;
        }
        const controller = new AbortController();
        const timer = window.setTimeout(async () => {
            const request = JSON.parse(key) as CreateTaskInput;
            try {
                const estimate = await http.post<Estimate>("/tasks/estimate", request, { signal: controller.signal });
                if (controller.signal.aborted) return;
                if (estimate.state !== "available" || !validPlatformPoints(estimate.points) || estimate.priceVersion !== request.platform?.expectedPriceVersion || estimate.variantId !== request.platform.variantId) {
                    setDisplay({ key, label: "估价暂不可用" });
                    return;
                }
                const label = `预计约 ${estimate.points} 积分`;
                if (recent.current.size >= 20) recent.current.delete(recent.current.keys().next().value!);
                recent.current.set(key, { label, expires: Date.now() + 30_000 });
                setDisplay({ key, label });
            } catch {
                if (!controller.signal.aborted) setDisplay({ key, label: "估价暂不可用" });
            }
        }, 250);
        return () => {
            window.clearTimeout(timer);
            controller.abort();
        };
    }, [key]);
    if (!key) return "";
    return display?.key === key ? display.label : "估价中";
}
