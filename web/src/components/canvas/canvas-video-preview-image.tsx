import type { ImgHTMLAttributes, ReactNode } from "react";

import { CachedResourceImage } from "@/components/cached-resource-image";
import { canvasNodeVideoPreviewReference } from "@/lib/canvas/canvas-media-preview";
import type { CanvasNodeData } from "@/types/canvas";
import { useUserStore } from "@/stores/use-user-store";

type CanvasVideoPreviewImageProps = Omit<ImgHTMLAttributes<HTMLImageElement>, "src"> & {
    node: CanvasNodeData;
    fallback?: ReactNode;
    loadingFallback?: ReactNode;
    eager?: boolean;
};

/**
 * Passive video poster image. Persisted posters are always resolved through
 * their resource storage key so remote media is served by OSS/CDN rather than
 * an obsolete platform proxy URL embedded in older canvas snapshots.
 */
export function CanvasVideoPreviewImage({ node, fallback = null, loadingFallback = fallback, eager = false, ...props }: CanvasVideoPreviewImageProps) {
    const platformIdentity = useUserStore((state) => state.platformIdentity);
    const preview = canvasNodeVideoPreviewReference(node, platformIdentity);
    if (!preview) return <>{fallback}</>;
    return <CachedResourceImage {...props} storageKey={preview.storageKey} src={preview.src} variant={preview.variant} fallback={fallback} loadingFallback={loadingFallback} eager={eager} />;
}
