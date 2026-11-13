import { sha256 } from "@noble/hashes/sha2.js";
import { bytesToHex } from "@noble/hashes/utils.js";
import { http, ApiError } from "./request";
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
    return `/api/materials/assets/${encodeURIComponent(id)}/versions/${encodeURIComponent(version)}/file?variant=${variant}`;
}
export function getPlatformMaterialStorage(signal?: AbortSignal) {
    return http.get<PlatformMaterialStorage>("/materials/storage", { signal });
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
    assertCurrent();
    let result = await http.post<Upload>(
        "/resources/platform-uploads",
        { commandId, filename: name, contentType: file.type || "application/octet-stream", byteLength: String(file.size), sha256: digest, width: meta?.width, height: meta?.height, durationMs: meta?.durationMs },
        { timeout: 35_000 },
    );
    assertCurrent();
    validUpload(result);
    const id = result.id;
    const uploadId = result.upload.upload_id;
    const path = `/resources/platform-uploads/${encodeURIComponent(id)}`;
    if (result.upload.state === "awaiting_upload") {
        const size = Number(result.upload.part_size);
        const tickets = [...(result.upload.tickets || [])].sort((a, b) => a.part_number - b.part_number);
        const count = Math.ceil(file.size / size);
        if (result.upload.byte_length !== String(file.size) || !Number.isSafeInteger(size) || size < 1 || count > 10000 || tickets.length !== count || tickets.some((ticket, i) => ticket.part_number !== i + 1)) throw new Error("上传分片响应不匹配");
        // Validate every ticket before disclosing any bytes.
        const headers = tickets.map(ticketHeaders);
        const parts: Array<{ part_number: number; etag: string }> = [];
        for (const [i, ticket] of tickets.entries()) {
            assertCurrent();
            const start = i * size;
            const end = Math.min(file.size, start + size);
            const response = await fetch(ticket.url, { method: "PUT", headers: headers[i], body: file.slice(start, end), credentials: "omit", redirect: "error", referrerPolicy: "no-referrer", signal: AbortSignal.timeout(120_000) });
            assertCurrent();
            if (!response.ok) throw new Error("文件分片上传失败，请恢复原上传");
            const etag = response.headers.get("ETag");
            if (!etag) throw new Error("上传服务未返回分片凭据，请检查跨域配置");
            parts.push({ part_number: ticket.part_number, etag });
            onProgress?.(end, file.size);
        }
        assertCurrent();
        try {
            result = await http.post<Upload>(`${path}/complete`, { parts }, { timeout: 35_000 });
        } catch (error) {
            assertCurrent();
            if (error instanceof ApiError && !error.retryable && error.status !== undefined) throw error;
            // Completion may already have committed. Read the same upload;
            // neither restart nor send another completion automatically.
            result = await http.get<Upload>(path, { timeout: 35_000 });
        }
    }
    for (let attempt = 0; attempt <= 60; attempt++) {
        assertCurrent();
        validUpload(result, id, uploadId);
        if (result.upload.state === "ready") {
            const asset = result.upload.asset;
            if (!result.resource && asset?.asset_id && asset.version?.asset_id === asset.asset_id && asset.version.version_id && asset.file?.sha256 === digest && asset.file.byte_length === String(file.size)) {
                result.resource = await adoptPlatformMaterial(asset.asset_id, asset.version.version_id, meta);
                assertCurrent();
            }
            if (!result.resource?.id || result.resource.status !== "ready" || result.resource.provider !== "platform") throw new Error("素材尚未完成采用，请查询原上传");
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
}
