import { getPlatformMaterials, platformMaterialFileURL, type PlatformMaterialAsset } from "@/services/api/platform-materials";
import { uploadResourceFile } from "@/services/api/resources";
import { readUploadImageSize } from "@/lib/canvas/canvas-file-upload";
import { probeMediaDurationMs } from "@/lib/media-metadata";
import { translate } from "@/i18n";
import type { LibraryAsset } from "@/pages/assets/asset-library-format";

export function platformLibraryAsset(item: PlatformMaterialAsset): LibraryAsset {
    const id = item.asset_id;
    const version = item.version.version_id;
    const file = (variant: Parameters<typeof platformMaterialFileURL>[2]) => platformMaterialFileURL(id, version, variant);
    const ready = item.state === "ready";
    const base = {
        id,
        primaryVersionId: version,
        title: item.title,
        tags: item.tags || [],
        category: "material" as const,
        status: ready ? ("confirmed" as const) : ("archived" as const),
        source: item.source_type === "generation" ? translate("library.assets.platform.generated") : item.source_type === "upload" ? translate("library.assets.platform.uploaded") : item.source_type,
        createdAt: item.created_at,
        updatedAt: item.updated_at,
        metadata: {
            platformMaterial: true,
            projectName: (item.projects || [])
                .map((project) => project.title)
                .filter(Boolean)
                .join("、"),
        },
    };
    const data = { bytes: Number(item.file.byte_length), mimeType: item.file.content_type };
    const size = { width: item.width || 0, height: item.height || 0 };
    switch (item.kind) {
        case "image":
            return { ...base, kind: "image", coverUrl: ready ? file("thumbnail") : "", data: { ...data, ...size, dataUrl: ready ? file("preview") : "" } };
        case "video":
            return { ...base, kind: "video", coverUrl: ready ? file("poster") : "", data: { ...data, ...size, durationMs: item.durationMs, url: ready ? file("playback") : "" } };
        case "audio":
            return { ...base, kind: "audio", coverUrl: "", data: { ...data, durationMs: item.durationMs, url: ready ? file("playback") : "" } };
        default:
            return { ...base, kind: "document", coverUrl: "", data: { ...data, url: "", fileName: item.title } };
    }
}

type LibraryPage = { assets: LibraryAsset[]; total: number; kindCounts?: Record<string, number>; categoryCounts?: Record<string, number>; folderCounts?: Record<string, number> };

export async function loadPlatformLibraryPage(options: { page: number; pageSize: number; kind?: string; source?: string; query?: string; trash?: boolean; signal?: AbortSignal }): Promise<LibraryPage> {
    const page = await getPlatformMaterials(options.page, options.query, options.kind, options.signal, { limit: options.pageSize, source: options.source, trash: options.trash });
    return { assets: page.items.map(platformLibraryAsset), total: Number(page.total) };
}

export async function uploadLibraryMaterial(file: File) {
    const kind = file.type.startsWith("image/") ? "image" : file.type.startsWith("video/") ? "video" : file.type.startsWith("audio/") ? "audio" : "file";
    const meta = kind === "image" ? await readUploadImageSize(file).catch(() => ({})) : { durationMs: await probeMediaDurationMs(file) };
    const resource = await uploadResourceFile(file, kind, { fileName: file.name, ...meta });
    if (!resource.material) throw new Error(translate("library.assets.detailFailed"));
    return resource.material;
}
