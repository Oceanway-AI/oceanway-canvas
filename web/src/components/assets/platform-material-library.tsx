import { useEffect, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { App, Button, Checkbox, Input, Select } from "antd";
import { FileText, Upload, RefreshCw } from "lucide-react";
import { useUserStore } from "@/stores/use-user-store";
import { getActiveUserScope } from "@/lib/user-scope";
import { getPlatformMaterials, getPlatformMaterialAccess, type PlatformMaterialAsset } from "@/services/api/platform-materials";
import { uploadResourceFile } from "@/services/api/resources";
import { preparePlatformMaterialInsert } from "@/services/platform-material-selection";
import type { InsertAssetPayload } from "@/components/canvas/asset-picker-modal";
import { AppModal } from "@/components/ui/product/app-modal";
import { saveAs } from "file-saver";

type Props = { onInsert?: (items: InsertAssetPayload[]) => Promise<unknown> | void; multiple?: boolean; compact?: boolean; actionLabel?: string };
const labels: Record<string, string> = { image: "图片", video: "视频", audio: "音频", document: "文档" };

export function PlatformMaterialLibrary({ onInsert, multiple = true, compact = false, actionLabel }: Props) {
    const userId = useUserStore((state) => state.user?.id);
    return <MaterialLibrary key={userId || "anonymous"} onInsert={onInsert} multiple={multiple} compact={compact} actionLabel={actionLabel} />;
}

function MaterialLibrary({ onInsert, multiple, compact, actionLabel }: Props) {
    const { message } = App.useApp();
    const userId = useUserStore((state) => state.user?.id);
    const queryClient = useQueryClient();
    const fileInput = useRef<HTMLInputElement>(null);
    const mounted = useRef(true);
    useEffect(() => {
        mounted.current = true;
        return () => {
            mounted.current = false;
        };
    }, []);
    const [page, setPage] = useState(1);
    const [kind, setKind] = useState("");
    const [search, setSearch] = useState("");
    const [selected, setSelected] = useState<PlatformMaterialAsset[]>([]);
    const [busy, setBusy] = useState(false);
    const [uploadStatus, setUploadStatus] = useState("");
    const [preview, setPreview] = useState<{ title: string; kind: string; url: string }>();
    const query = useQuery({ queryKey: ["platform-materials", userId, page, search, kind], queryFn: ({ signal }) => getPlatformMaterials(page, search, kind, signal), enabled: !!userId, retry: false });
    const items = query.data?.items || [];
    const operate = async (action: (assertCurrent: () => void) => Promise<void>) => {
        if (busy) return;
        const scope = getActiveUserScope();
        const assertCurrent = () => {
            if (!mounted.current || getActiveUserScope() !== scope) throw new DOMException("素材操作已取消", "AbortError");
        };
        setBusy(true);
        try {
            await action(assertCurrent);
        } catch (error) {
            if (mounted.current && !(error instanceof DOMException && error.name === "AbortError")) message.error(error instanceof Error ? error.message : "素材操作失败");
        } finally {
            if (mounted.current) {
                setBusy(false);
                setUploadStatus("");
            }
        }
    };
    const insert = () =>
        operate(async (assertCurrent) => {
            const payloads: InsertAssetPayload[] = [];
            for (const asset of selected) {
                assertCurrent();
                payloads.push(await preparePlatformMaterialInsert(asset));
            }
            assertCurrent();
            await onInsert?.(payloads);
            if (mounted.current) setSelected([]);
        });
    const view = (asset: PlatformMaterialAsset, download = false) =>
        operate(async (assertCurrent) => {
            const access = await getPlatformMaterialAccess(asset, download);
            assertCurrent();
            if (download) {
                const response = await fetch(access.url, { credentials: "omit", redirect: "error", referrerPolicy: "no-referrer" });
                if (!response.ok) throw new Error("素材下载失败");
                const blob = await response.blob();
                assertCurrent();
                saveAs(blob, asset.title);
                return;
            }
            setPreview({ title: asset.title, kind: asset.kind, url: access.url });
        });
    const upload = (files: File[]) =>
        operate(async (assertCurrent) => {
            for (const file of files) {
                assertCurrent();
                setUploadStatus(`正在准备 ${file.name}`);
                const fileKind = file.type.startsWith("image/") ? "image" : file.type.startsWith("video/") ? "video" : file.type.startsWith("audio/") ? "audio" : "file";
                await uploadResourceFile(file, fileKind, { fileName: file.name }, (loaded, total) => {
                    if (mounted.current) setUploadStatus(loaded === total ? `正在确认 ${file.name}` : `正在上传 ${file.name} · ${Math.round((100 * loaded) / total)}%`);
                });
                assertCurrent();
                await queryClient.invalidateQueries({ queryKey: ["platform-materials", userId] });
                await queryClient.invalidateQueries({ queryKey: ["platform-storage", userId] });
            }
            message.success("素材已保存");
        });
    return (
        <div className="flex min-h-0 flex-1 flex-col gap-3 p-3">
            <div className="flex flex-wrap items-center gap-2">
                <Input.Search
                    aria-label="搜索平台素材"
                    placeholder="搜索素材"
                    allowClear
                    onSearch={(value) => {
                        setSearch(value.trim());
                        setPage(1);
                    }}
                    className={compact ? "w-full" : "max-w-xs"}
                />
                <Select
                    aria-label="素材类型"
                    value={kind}
                    onChange={(value) => {
                        setKind(value);
                        setPage(1);
                    }}
                    options={[{ value: "", label: "全部类型" }, ...Object.entries(labels).map(([value, label]) => ({ value, label }))]}
                />
                <Button aria-label="刷新素材" icon={<RefreshCw className="size-4" />} loading={query.isFetching} onClick={() => void query.refetch()} />
                <Button icon={<Upload className="size-4" />} disabled={busy} onClick={() => fileInput.current?.click()}>
                    上传
                </Button>
                <input
                    hidden
                    ref={fileInput}
                    type="file"
                    multiple
                    onChange={(event) => {
                        const files = Array.from(event.target.files || []);
                        event.target.value = "";
                        void upload(files);
                    }}
                />
            </div>
            {uploadStatus ? (
                <p role="status" className="text-sm text-muted-foreground">
                    {uploadStatus}
                </p>
            ) : null}
            {!userId ? <p>登录后查看素材。</p> : query.isPending ? <p role="status">正在加载素材…</p> : null}
            {query.isError ? (
                <div role="alert">
                    素材读取失败。<Button onClick={() => void query.refetch()}>重试</Button>
                </div>
            ) : null}
            <div className="thin-scrollbar min-h-0 flex-1 space-y-2 overflow-y-auto">
                {items.map((asset) => {
                    const insertable = ["image", "video", "audio"].includes(asset.kind) && asset.state === "ready";
                    const checked = selected.some((v) => v.asset_id === asset.asset_id && v.version.version_id === asset.version.version_id);
                    return (
                        <article key={`${asset.asset_id}:${asset.version.version_id}`} className="flex items-center gap-3 rounded-lg border border-border bg-surface p-3">
                            {onInsert ? (
                                <Checkbox
                                    aria-label={`选择 ${asset.title}`}
                                    checked={checked}
                                    disabled={!insertable || busy}
                                    onChange={() => setSelected((current) => (checked ? current.filter((v) => v.asset_id !== asset.asset_id) : multiple ? [...current, asset] : [asset]))}
                                />
                            ) : (
                                <FileText className="size-5 shrink-0 text-muted-foreground" />
                            )}
                            <div className="min-w-0 flex-1">
                                <p className="truncate text-sm font-medium" title={asset.title}>
                                    {asset.title}
                                </p>
                                <p className="text-xs text-muted-foreground">
                                    {labels[asset.kind] || "文件"}
                                    {asset.state !== "ready" ? " · 暂不可用" : ""}
                                </p>
                            </div>
                            {!compact && insertable ? (
                                <Button size="small" disabled={busy} onClick={() => void view(asset)}>
                                    预览
                                </Button>
                            ) : null}
                            {!onInsert ? (
                                <Button size="small" disabled={busy || asset.state !== "ready"} onClick={() => void view(asset, true)}>
                                    下载
                                </Button>
                            ) : null}
                        </article>
                    );
                })}
                {query.isSuccess && !items.length ? <p className="py-8 text-center text-sm text-muted-foreground">没有匹配的素材。上传文件后即可在画布使用。</p> : null}
            </div>
            <div className="flex flex-wrap items-center gap-2">
                <Button disabled={page === 1 || query.isFetching} onClick={() => setPage(page - 1)}>
                    上一页
                </Button>
                <span className="text-sm">
                    第 {page} 页 · 共 {query.data?.total || "0"} 项
                </span>
                <Button disabled={query.isFetching || !query.data || BigInt(page * 50) >= BigInt(query.data.total)} onClick={() => setPage(page + 1)}>
                    下一页
                </Button>
                {onInsert ? (
                    <Button type="primary" disabled={!selected.length || busy} loading={busy && !uploadStatus} onClick={() => void insert()}>
                        {actionLabel || "插入"} {selected.length || ""} 项
                    </Button>
                ) : null}
            </div>
            <AppModal title={preview?.title} open={!!preview} onCancel={() => setPreview(undefined)} footer={null}>
                {preview?.kind === "image" ? (
                    <img src={preview.url} alt={preview.title} crossOrigin="anonymous" className="max-h-[70vh] w-full object-contain" />
                ) : preview?.kind === "video" ? (
                    <video src={preview.url} controls crossOrigin="anonymous" className="max-h-[70vh] w-full" />
                ) : preview ? (
                    <audio src={preview.url} controls crossOrigin="anonymous" />
                ) : null}
            </AppModal>
        </div>
    );
}
