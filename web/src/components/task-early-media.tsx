import { useEffect, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { earlyPreviewURL, type TaskEarlyPreview } from "@/services/task-early-previews";
import { waitForResourcePreview } from "@/services/api/resources";

/** A playing video keeps its source until pause/end, even when archival finishes. */
export function TaskEarlyMedia({
    previews,
    show,
    includeChildren,
    playbackKey,
    preload = "metadata",
    children,
}: {
    previews: TaskEarlyPreview[];
    show: boolean;
    includeChildren?: boolean;
    playbackKey?: string;
    preload?: "metadata" | "none";
    children?: ReactNode;
}) {
    const { t } = useTranslation();
    const [playing, setPlaying] = useState<TaskEarlyPreview>();
    const [failed, setFailed] = useState<Record<string, boolean>>({});
    const [settledKey, setSettledKey] = useState("");
    useEffect(() => {
        if (!playbackKey) return;
        const controller = new AbortController();
        const settled = () => {
            if (!controller.signal.aborted) setSettledKey(playbackKey);
        };
        void waitForResourcePreview(playbackKey, "playback", controller.signal).then(settled, settled);
        return () => controller.abort();
    }, [playbackKey]);
    let visible = show || (playbackKey && playbackKey !== settledKey) ? previews : [];
    if (playing && !visible.some((ref) => ref.taskId === playing.taskId && ref.index === playing.index)) visible = [...visible, playing];
    if (!visible.length) return children;
    return (
        <div className="flex size-full flex-wrap items-center justify-center gap-2" onPointerDown={(event) => event.stopPropagation()}>
            {includeChildren && children}
            {visible.map((ref) => {
                const src = earlyPreviewURL(ref);
                if (!src) return null;
                if (failed[src])
                    return (
                        <span key={src} role="status">
                            {t("library.tasks.row.waitingForArchivedPreview")}{" "}
                            <button type="button" onClick={() => setFailed((current) => ({ ...current, [src]: false }))}>
                                {t("common.retry")}
                            </button>
                        </span>
                    );
                const onError = () => {
                    setPlaying(undefined);
                    setFailed((current) => ({ ...current, [src]: true }));
                };
                return ref.kind === "video" ? (
                    <video
                        key={src}
                        src={src}
                        controls
                        controlsList="nodownload"
                        playsInline
                        preload={preload}
                        className="max-h-full max-w-full object-contain"
                        onPlay={() => setPlaying(ref)}
                        onPause={() => setPlaying(undefined)}
                        onEnded={() => setPlaying(undefined)}
                        onError={onError}
                    />
                ) : (
                    <img key={src} src={src} alt={t("library.tasks.row.previewImage")} className="max-h-full max-w-full object-contain" onError={onError} />
                );
            })}
        </div>
    );
}
