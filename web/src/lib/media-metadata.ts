// 上传前从 <video>/<audio> 读取媒体时长和视频尺寸。
// 结果随上传 meta 传给后端，供资源摘要和时间线片段使用；这里不负责验证完整播放能力。

const PROBE_TIMEOUT_MS = 8000;

function isProbeable(file: File): boolean {
    return /^video\//.test(file.type) || /^audio\//.test(file.type);
}

type MediaMetadata = { durationMs?: number; width?: number; height?: number };

/** Metadata is optional: a codec that the browser cannot read must not prevent an upload. */
export async function probeMediaMetadata(file: File): Promise<MediaMetadata> {
    if (!isProbeable(file)) return {};
    const url = URL.createObjectURL(file);
    const el: HTMLVideoElement | HTMLAudioElement = /^video\//.test(file.type)
        ? document.createElement("video")
        : document.createElement("audio");
    let timeoutId: number | undefined;
    el.preload = "metadata";
    // 某些浏览器只有在静音时才允许视频元素主动加载元数据；这里只禁止声音，不触发自动播放。
    el.muted = true;
    try {
        return await new Promise<MediaMetadata>((resolve) => {
            timeoutId = window.setTimeout(() => resolve({}), PROBE_TIMEOUT_MS);
            el.onloadedmetadata = () => {
                const durationMs = Number.isFinite(el.duration) ? Math.round(el.duration * 1000) : 0;
                const meta: MediaMetadata = durationMs > 0 ? { durationMs } : {};
                if (el instanceof HTMLVideoElement && el.videoWidth > 0 && el.videoHeight > 0) {
                    meta.width = el.videoWidth;
                    meta.height = el.videoHeight;
                }
                resolve(meta);
            };
            el.onerror = () => resolve({});
            el.src = url;
        });
    } catch {
        return {};
    } finally {
        if (timeoutId !== undefined) window.clearTimeout(timeoutId);
        el.onloadedmetadata = null;
        el.onerror = null;
        el.removeAttribute("src");
        el.load();
        URL.revokeObjectURL(url);
    }
}

export async function probeMediaDurationMs(file: File): Promise<number | undefined> {
    return (await probeMediaMetadata(file)).durationMs;
}
