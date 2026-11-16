import { sha256 } from "@noble/hashes/sha2.js";
import { translate } from "@/i18n";
import { bytesToHex } from "@noble/hashes/utils.js";
import { getActiveUserScope, scopedStorageKey } from "@/lib/user-scope";
import { http, ApiError, apiBaseURL } from "./request";
import type { RemoteResource, ResourceUploadMeta } from "./resources";

export type PlatformMaterialAsset = {
    asset_id: string;
    title: string;
    kind: string;
    state: string;
    current_version_id: string;
    tags: string[];
    created_at: string;
    updated_at: string;
    source_type: string;
    projects: { app: string; project_id: string; title: string }[];
    width?: number;
    height?: number;
    durationMs?: number;
    version: { version_id: string; asset_id: string; file_id: string };
    file: { file_id: string; byte_length: string; content_type: string; sha256: string };
};
export type PlatformMaterialStorage = { limit_bytes: string; used_bytes: string; reserved_bytes: string; remaining_bytes: string; over_limit: boolean };
type Ticket = { part_number: number; method: string; url: string; headers: Record<string, string>; expires_at: string };
type Upload = {
    id: string;
    upload: { protocol: string; upload_id: string; state: string; byte_length?: string; part_size?: string; asset?: PlatformMaterialAsset; tickets?: Ticket[]; retry_after_seconds?: number };
    resource?: RemoteResource;
};
type UploadCheckpoint = {
    fingerprint: string;
    filename: string;
    id?: string;
    uploadId?: string;
    partSize?: number;
    parts: Array<{ part_number: number; etag: string }>;
    meta: Pick<ResourceUploadMeta, "width" | "height" | "durationMs">;
};
const activeUploads = new Map<string, { fingerprint: string; promise: Promise<RemoteResource & { material?: PlatformMaterialAsset }> }>();

function readUploadCheckpoint(key: string, fingerprint: string, filename: string, meta?: ResourceUploadMeta): UploadCheckpoint {
    const raw = typeof window === "undefined" ? null : window.localStorage.getItem(key);
    if (!raw) return { fingerprint, filename, parts: [], meta: { width: meta?.width, height: meta?.height, durationMs: meta?.durationMs } };
    let value: UploadCheckpoint;
    try {
        value = JSON.parse(raw) as UploadCheckpoint;
    } catch {
        throw new ApiError(translate("canvasA.fileUpload.recoveryInvalid"), { status: 409 });
    }
    if (
        !value ||
        typeof value !== "object" ||
        typeof value.fingerprint !== "string" ||
        typeof value.filename !== "string" ||
        !value.filename ||
        !value.meta ||
        typeof value.meta !== "object" ||
        Array.isArray(value.meta) ||
        Object.entries(value.meta).some(([key, number]) => !["width", "height", "durationMs"].includes(key) || typeof number !== "number" || !Number.isFinite(number) || number < 0) ||
        Boolean(value.id) !== Boolean(value.uploadId) ||
        (value.id && (typeof value.id !== "string" || typeof value.uploadId !== "string")) ||
        (value.partSize !== undefined && (!Number.isSafeInteger(value.partSize) || value.partSize < 1)) ||
        !Array.isArray(value.parts) ||
        value.parts.length > 10000 ||
        value.parts.some((part) => !part || !Number.isSafeInteger(part.part_number) || part.part_number < 1 || typeof part.etag !== "string" || !part.etag || part.etag.length > 512) ||
        new Set(value.parts.map((part) => part.part_number)).size !== value.parts.length
    )
        throw new ApiError(translate("canvasA.fileUpload.recoveryInvalid"), { status: 409 });
    if (value.fingerprint !== fingerprint) throw new ApiError(translate("canvasA.fileUpload.resumeOriginal"), { status: 409 });
    return value;
}

function saveUploadCheckpoint(key: string, value: UploadCheckpoint) {
    // Only the original identity, file fingerprint and acknowledged ETags survive closure.
    try {
        if (typeof window !== "undefined") window.localStorage.setItem(key, JSON.stringify(value));
    } catch {
        throw new ApiError(translate("canvasA.fileUpload.progressStorageFailed"), { status: 409 });
    }
}

export async function getPlatformMaterials(page = 1, query = "", kind = "", signal?: AbortSignal, options: { limit?: number; source?: string; trash?: boolean } = {}) {
    return http.get<{ items: PlatformMaterialAsset[]; total: string; page: number; limit: number }>("/materials/assets", {
        signal,
        params: { page, limit: options.limit || 50, q: query || undefined, kind: kind || undefined, source_type: options.source || undefined, trash: options.trash || undefined },
    });
}
export function patchPlatformMaterial(id: string, patch: { title?: string; tags?: string[]; state?: "ready" | "trashed" }) {
    return http.patch<PlatformMaterialAsset>(`/materials/assets/${encodeURIComponent(id)}`, patch);
}
export function deletePlatformMaterial(id: string) {
    return http.delete(`/materials/assets/${encodeURIComponent(id)}`);
}
export function platformMaterialFileURL(id: string, version: string, variant: "thumbnail" | "poster" | "preview" | "playback" | "original") {
    return `${apiBaseURL.replace(/\/+$/, "")}/materials/assets/${encodeURIComponent(id)}/versions/${encodeURIComponent(version)}/file?variant=${variant}`;
}
export function getPlatformMaterialStorage(signal?: AbortSignal) {
    return http.get<PlatformMaterialStorage>("/materials/storage", { signal });
}

export async function getPlatformMaterialAccess(asset: PlatformMaterialAsset, download = false, signal?: AbortSignal) {
    const variant = download ? "original" : asset.kind === "video" || asset.kind === "audio" ? "playback" : "preview";
    const result = await http.post<{ protocol: string; results: Array<{ item_id: string; status: string; asset_id: string; version_id: string; file_id: string; requested_variant: string; actual_variant: string; url: string; expires_at: string }> }>(
        "/materials/access",
        { items: [{ item_id: "view", asset_id: asset.asset_id, version_id: asset.version.version_id, purpose: download ? "download" : "display", variant }] },
        { signal },
    );
    const access = result.results?.[0];
    if (
        result.protocol !== "oceanway.materials.v2" ||
        result.results.length !== 1 ||
        !access ||
        access.status !== "ready" ||
        access.item_id !== "view" ||
        access.asset_id !== asset.asset_id ||
        access.version_id !== asset.version.version_id ||
        access.file_id !== asset.file.file_id ||
        access.actual_variant !== variant ||
        !Number.isFinite(Date.parse(access.expires_at)) ||
        Date.parse(access.expires_at) <= Date.now()
    )
        throw new Error("素材暂时不可访问，请稍后重试");
    const url = new URL(access.url);
    if (url.protocol !== "https:" || url.username || url.password || url.hash) throw new Error("素材访问地址无效");
    return access;
}
export async function adoptPlatformMaterial(assetId: string, versionId: string, meta?: ResourceUploadMeta) {
    const data = await http.post<{ resource: RemoteResource }>("/resources/adopt", { assetId, versionId, width: meta?.width, height: meta?.height, durationMs: meta?.durationMs });
    return data.resource;
}

async function fileDigest(file: Blob, assertCurrent: () => void) {
    const digest = sha256.create();
    // Incremental hashing keeps large video uploads bounded in browser memory.
    for (let offset = 0; offset < file.size; offset += 4 << 20) {
        assertCurrent();
        digest.update(new Uint8Array(await file.slice(offset, offset + (4 << 20)).arrayBuffer()));
    }
    assertCurrent();
    return bytesToHex(digest.digest());
}

function validUpload(data: Upload, id?: string, uploadId?: string) {
    if (!data?.id || data.upload?.protocol !== "oceanway.materials.v2" || !data.upload.upload_id || (id && data.id !== id) || (uploadId && data.upload.upload_id !== uploadId)) throw new Error("上传状态响应不匹配");
}

function ticketHeaders(ticket: Ticket) {
    const url = new URL(ticket.url);
    // A loopback Studio preview can relay a signed PUT to its private TLS
    // object store. Only its own upload route may represent a local ticket.
    const localUpload = url.protocol === "http:" && url.hostname === "127.0.0.1" && url.origin === globalThis.location?.origin && url.pathname.startsWith("/_local-material-upload/");
    if ((!localUpload && url.protocol !== "https:") || url.username || url.password || url.hash || ticket.method !== "PUT" || !Number.isFinite(Date.parse(ticket.expires_at)) || Date.parse(ticket.expires_at) <= Date.now())
        throw new Error("上传票据无效或已过期");
    const headers = new Headers(ticket.headers);
    for (const name of headers.keys()) {
        if (/^(cookie|authorization|proxy-authorization|host)$|^x-platform-|^x-oceanway-/i.test(name)) throw new Error("上传票据包含不允许的身份字段");
    }
    return headers;
}

export async function uploadPlatformResource(file: Blob, name: string, meta: ResourceUploadMeta | undefined, assertCurrent: () => void, onProgress?: (loaded: number, total: number) => void): Promise<RemoteResource & { material?: PlatformMaterialAsset }> {
    if (!Number.isSafeInteger(file.size) || file.size < 1) throw new ApiError("不能上传空文件", { status: 400 });
    const digest = await fileDigest(file, assertCurrent);
    const commandId = bytesToHex(sha256(new TextEncoder().encode(meta?.idempotencyKey || `${digest}\n${name}\n${file.type}`)));
    const owner = getActiveUserScope();
    const key = scopedStorageKey(`studio-material-upload:v1:${commandId}`, owner);
    const fingerprint = JSON.stringify([digest, file.size, file.type]);
    const existing = activeUploads.get(key);
    if (existing) {
        if (existing.fingerprint !== fingerprint) throw new ApiError(translate("canvasA.fileUpload.resumeOriginal"), { status: 409 });
        const result = await existing.promise;
        assertCurrent();
        return result;
    }
    const resume = async () => {
        assertCurrent();
        const checkpoint = readUploadCheckpoint(key, fingerprint, name, meta);
        saveUploadCheckpoint(key, checkpoint);
        let result = checkpoint.id
            ? await http.get<Upload>(`/resources/platform-uploads/${encodeURIComponent(checkpoint.id)}`, { timeout: 35_000 })
            : await http.post<Upload>(
                  "/resources/platform-uploads",
                  { commandId, filename: checkpoint.filename, contentType: file.type || "application/octet-stream", byteLength: String(file.size), sha256: digest, ...checkpoint.meta },
                  { timeout: 35_000 },
              );
        assertCurrent();
        validUpload(result, checkpoint.id, checkpoint.uploadId);
        checkpoint.id = result.id;
        checkpoint.uploadId = result.upload.upload_id;
        saveUploadCheckpoint(key, checkpoint);
        const path = `/resources/platform-uploads/${encodeURIComponent(result.id)}`;
        if (result.upload.state === "awaiting_upload") {
            const size = Number(result.upload.part_size);
            const tickets = [...(result.upload.tickets || [])].sort((a, b) => a.part_number - b.part_number);
            const count = Math.ceil(file.size / size);
            if (
                result.upload.byte_length !== String(file.size) ||
                !Number.isSafeInteger(size) ||
                size < 1 ||
                count > 10000 ||
                tickets.length !== count ||
                tickets.some((ticket, i) => ticket.part_number !== i + 1) ||
                (checkpoint.partSize !== undefined && checkpoint.partSize !== size) ||
                checkpoint.parts.some((part) => part.part_number > count)
            )
                throw new Error("上传分片响应不匹配");
            checkpoint.partSize = size;
            const saved = new Set(checkpoint.parts.map((part) => part.part_number));
            const missing = tickets.filter((ticket) => !saved.has(ticket.part_number));
            const headers = missing.map(ticketHeaders);
            let confirmedBytes = checkpoint.parts.reduce((total, part) => total + Math.min(size, file.size - (part.part_number - 1) * size), 0);
            if (confirmedBytes) onProgress?.(confirmedBytes, file.size);
            for (const [i, ticket] of missing.entries()) {
                assertCurrent();
                const start = (ticket.part_number - 1) * size;
                const end = Math.min(file.size, start + size);
                const response = await fetch(ticket.url, { method: "PUT", headers: headers[i], body: file.slice(start, end), credentials: "omit", redirect: "error", referrerPolicy: "no-referrer", signal: AbortSignal.timeout(120_000) });
                assertCurrent();
                if (!response.ok) throw new Error("文件分片上传失败，请恢复原上传");
                const etag = response.headers.get("ETag");
                if (!etag || etag.length > 512) throw new Error("上传服务未返回分片凭据，请检查跨域配置");
                checkpoint.parts.push({ part_number: ticket.part_number, etag });
                checkpoint.parts.sort((left, right) => left.part_number - right.part_number);
                saveUploadCheckpoint(key, checkpoint);
                confirmedBytes += end - start;
                onProgress?.(confirmedBytes, file.size);
            }
            assertCurrent();
            try {
                result = await http.post<Upload>(`${path}/complete`, { parts: checkpoint.parts }, { timeout: 35_000 });
            } catch (error) {
                assertCurrent();
                if (error instanceof ApiError && !error.retryable && error.status !== undefined) throw error;
                result = await http.get<Upload>(path, { timeout: 35_000 });
            }
        }
        for (let attempt = 0; attempt <= 60; attempt++) {
            assertCurrent();
            validUpload(result, checkpoint.id, checkpoint.uploadId);
            if (result.upload.state === "ready") {
                const asset = result.upload.asset;
                if (!result.resource && asset?.asset_id && asset.version?.asset_id === asset.asset_id && asset.version.version_id && asset.file?.sha256 === digest && asset.file.byte_length === String(file.size)) {
                    result.resource = await adoptPlatformMaterial(asset.asset_id, asset.version.version_id, checkpoint.meta);
                    assertCurrent();
                }
                if (!result.resource?.id || result.resource.status !== "ready" || result.resource.provider !== "platform") throw new Error("素材尚未完成采用，请查询原上传");
                // Keep the original identity until its caller has durably adopted the result.
                checkpoint.parts = [];
                saveUploadCheckpoint(key, checkpoint);
                return { ...result.resource, material: asset };
            }
            if (result.upload.state === "failed" || result.upload.state === "expired") throw new ApiError("原上传失败或已过期，请检查文件后重新上传", { status: 422 });
            if (!["verifying", "sealing"].includes(result.upload.state)) throw new Error("上传状态待确认，请恢复原上传");
            if (attempt === 60) break;
            await new Promise((resolve) => setTimeout(resolve, Math.max(1, Math.min(10, result.upload.retry_after_seconds || 1)) * 1000));
            assertCurrent();
            result = await http.get<Upload>(path, { timeout: 35_000 });
        }
        throw new Error("素材仍在处理中，请稍后恢复原上传");
    };
    const work = typeof navigator !== "undefined" && navigator.locks ? navigator.locks.request(key, resume) : resume();
    const promise = work.finally(() => activeUploads.delete(key));
    activeUploads.set(key, { fingerprint, promise });
    return promise;
}
