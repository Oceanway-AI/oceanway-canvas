import { App, Button } from "antd";
import { AlertCircle, Bug, Copy, Maximize2, RefreshCw, RotateCcw, X } from "lucide-react";
import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";

import { MediaPreview } from "@/components/media-preview";
import { AppModal } from "@/components/ui/product/app-modal";
import { RelativeTime } from "@/components/ui/product/relative-time";
import { currentLocale, translate } from "@/i18n";
import { generationErrorMessage } from "@/lib/generation-error";
import { formatTaskKind, generationTaskShowsProgress, generationTaskStageLabel, generationTaskStatusLabel, mediaDeliverySummary } from "@/lib/generation-task-display";
import { progressFillStyle } from "@/lib/progress-fill";
import { getActiveUserScope } from "@/lib/user-scope";
import { getResourceAccess, resolveResourceAccessURL, type ResourceAccessVariant } from "@/services/api/resources";
import { formatTaskLog, type GenerationTask, type TaskLog } from "@/services/api/task-center";
import { taskResultMedia, taskResultMediaAccess, type TaskResultMedia } from "./task-result-media";
import { isTaskActive, isTaskFailed, isTaskRetryBlocked, providerCancelStatusLabel, taskAttentionReason, TaskBilling, taskKindIcon, TaskStatusPill, taskTone } from "./task-shared";

type TaskDetailModalProps = {
    task: GenerationTask | null;
    loading: boolean;
    logs: TaskLog[];
    logsLoading: boolean;
    actingId: string;
    canvasName: string;
    modelName: string;
    creditsEnabled: boolean;
    onClose: () => void;
    onRetry: (task: GenerationTask) => void;
    onQueryProvider: (task: GenerationTask) => void;
    onExportDiagnostics: (task: GenerationTask) => void;
    onPreviewMedia: (media: { url: string; kind: "image" | "video"; title: string }) => void;
};

/**
 * 任务详情弹窗：左侧是结果、提示词和运行日志，右侧是任务信息、参数和原始结果；操作集中在底栏。
 * 打开时先用列表里的摘要渲染，详情和日志到达后原位替换。
 */
export function TaskDetailModal({ task, loading, logs, logsLoading, actingId, canvasName, modelName, creditsEnabled, onClose, onRetry, onQueryProvider, onExportDiagnostics, onPreviewMedia }: TaskDetailModalProps) {
    const { t } = useTranslation();
    const { message } = App.useApp();
    const copyText = async (value: string) => {
        try {
            await navigator.clipboard.writeText(value);
            message.success(t("library.tasks.detail.copied"));
        } catch {
            message.error(t("library.tasks.detail.copyFailed"));
        }
    };

    return (
        <AppModal open={Boolean(task)} onCancel={onClose} footer={null} title={null} closable={false} centered width="min(1080px, calc(100vw - 32px))" rootClassName="task-detail-modal-root" flush>
            {task ? (
                <TaskDetailContent
                    key={task.id}
                    task={task}
                    loading={loading}
                    logs={logs}
                    logsLoading={logsLoading}
                    acting={actingId === task.id}
                    canvasName={canvasName}
                    modelName={modelName}
                    creditsEnabled={creditsEnabled}
                    onClose={onClose}
                    onRetry={() => onRetry(task)}
                    onQueryProvider={() => onQueryProvider(task)}
                    onExportDiagnostics={() => onExportDiagnostics(task)}
                    onPreviewMedia={onPreviewMedia}
                    onCopy={(value) => void copyText(value)}
                />
            ) : null}
        </AppModal>
    );
}

function TaskDetailContent({
    task,
    loading,
    logs,
    logsLoading,
    acting,
    canvasName,
    modelName,
    creditsEnabled,
    onClose,
    onRetry,
    onQueryProvider,
    onExportDiagnostics,
    onPreviewMedia,
    onCopy,
}: Omit<TaskDetailModalProps, "task" | "actingId" | "onRetry" | "onQueryProvider" | "onExportDiagnostics"> & {
    task: GenerationTask;
    acting: boolean;
    onRetry: () => void;
    onQueryProvider: () => void;
    onExportDiagnostics: () => void;
    onCopy: (value: string) => void;
}) {
    const { t } = useTranslation();
    const KindIcon = taskKindIcon(task);
    const active = isTaskActive(task);
    const failed = isTaskFailed(task);
    const retryBlocked = isTaskRetryBlocked(task);
    const stageLabel = generationTaskStageLabel(task);
    const media = taskResultMedia(task);
    const title = task.prompt || t("library.tasks.row.untitled");
    const parameters = taskParameterFields(task.inputJson);
    const platformMedia = task.provider === "platform" && (task.type === "canvas_image" || task.type === "canvas_video");
    const facts: Array<{ label: string; value: string }> = [
        { label: t("library.tasks.detail.status"), value: generationTaskStatusLabel(task) },
        ...(task.mediaStage ? [{ label: t("library.tasks.detail.delivery"), value: mediaDeliverySummary(task.status, task.mediaStage) }] : []),
        { label: t("library.tasks.detail.canvas"), value: canvasName },
        { label: t("library.tasks.detail.kind"), value: formatTaskKind(task) },
        { label: t("library.tasks.detail.model"), value: modelName },
        {
            label: t(platformMedia ? "library.tasks.detail.processingCount" : "library.tasks.detail.attempts"),
            value: t(platformMedia ? "library.tasks.detail.processingCountValue" : "library.tasks.detail.attemptValue", { count: platformMedia ? task.attempts || 0 : task.attempts || 1 }),
        },
        { label: t("library.tasks.detail.createdAt"), value: formatDate(task.createdAt) },
        { label: t("library.tasks.detail.startedAt"), value: formatDate(task.startedAt) },
        { label: t("library.tasks.detail.completedAt"), value: formatDate(task.completedAt) },
        { label: t("library.tasks.detail.duration"), value: formatTaskDuration(task) },
        ...(task.providerCancelStatus ? [{ label: t("library.tasks.detail.providerCancel"), value: providerCancelStatusLabel(task) }] : []),
        ...(task.providerCancelRequestedAt ? [{ label: t("library.tasks.detail.cancelRequestedAt"), value: formatDate(task.providerCancelRequestedAt) }] : []),
    ];

    return (
        <div className="task-detail" data-tone={taskTone(task)}>
            <header className="task-detail-header">
                <span className="task-detail-icon" aria-hidden="true">
                    <KindIcon />
                </span>
                <div className="task-detail-heading">
                    <h2 title={title}>{title}</h2>
                    <div className="task-detail-sub">
                        <TaskStatusPill task={task} />
                        <span>{formatTaskKind(task)}</span>
                        <span>{modelName}</span>
                        <RelativeTime value={task.createdAt} />
                    </div>
                </div>
                <button type="button" className="task-detail-close" aria-label={t("common.close")} onClick={onClose}>
                    <X />
                </button>
            </header>

            <div className="task-detail-body">
                <div className="task-detail-main">
                    {failed ? (
                        <div className="task-detail-alert" role="alert">
                            <AlertCircle aria-hidden="true" />
                            <div>
                                <strong>{t("library.tasks.detail.failedReason")}</strong>
                                <p>{task.error ? generationErrorMessage(task.error) : taskAttentionReason(task)}</p>
                            </div>
                        </div>
                    ) : null}

                    {active ? (
                        <div className="task-detail-progress" role="progressbar" aria-label={stageLabel} aria-valuemin={0} aria-valuemax={100} aria-valuenow={task.progress || 0}>
                            <span>
                                <span>{stageLabel}</span>
                                {generationTaskShowsProgress(task) ? <strong>{task.progress || 0}%</strong> : null}
                            </span>
                            <i>
                                <b style={progressFillStyle(generationTaskShowsProgress(task) ? task.progress || 0 : 0)} />
                            </i>
                        </div>
                    ) : null}

                    {media.length ? (
                        <section className="task-detail-section">
                            <h3>{t("library.tasks.detail.generated")}</h3>
                            <TaskResultGallery items={media} title={title} onPreview={onPreviewMedia} />
                        </section>
                    ) : task.status === "succeeded" && /image|video/.test(task.type) ? (
                        <section className="task-detail-section">
                            <h3>{t("library.tasks.detail.generated")}</h3>
                            <div className="task-detail-empty">{t("library.tasks.detail.noMedia")}</div>
                        </section>
                    ) : null}

                    <section className="task-detail-section">
                        <div className="task-detail-section-head">
                            <h3>{t("library.tasks.detail.prompt")}</h3>
                            {task.prompt ? (
                                <button type="button" className="task-detail-text-action" onClick={() => onCopy(task.prompt)}>
                                    <Copy aria-hidden="true" />
                                    {t("library.tasks.detail.copyPrompt")}
                                </button>
                            ) : null}
                        </div>
                        <p className="task-detail-prompt">{loading && !task.prompt ? t("library.tasks.detail.loading") : task.prompt || t("library.tasks.detail.none")}</p>
                    </section>

                    <section className="task-detail-section">
                        <h3>{t("library.tasks.detail.logs")}</h3>
                        {logsLoading ? (
                            <div className="task-detail-empty">{t("library.tasks.detail.logsLoading")}</div>
                        ) : logs.length ? (
                            <ol className="task-detail-logs">
                                {logs.map((log, index) => (
                                    <li key={`${log.createdAt}-${index}`} data-level={log.level}>
                                        <time dateTime={log.createdAt}>{formatLogTime(log.createdAt)}</time>
                                        <span className="task-detail-log-level">{log.level.toUpperCase()}</span>
                                        <span className="task-detail-log-message">{formatTaskLog(log)}</span>
                                    </li>
                                ))}
                            </ol>
                        ) : (
                            <div className="task-detail-empty">{t("library.tasks.detail.noLogs")}</div>
                        )}
                    </section>
                </div>

                <aside className="task-detail-side">
                    <section className="task-detail-section">
                        <h3>{t("library.tasks.detail.info")}</h3>
                        <dl className="task-detail-facts">
                            {facts.map((fact) => (
                                <div key={fact.label}>
                                    <dt>{fact.label}</dt>
                                    <dd title={fact.value}>{fact.value}</dd>
                                </div>
                            ))}
                            {creditsEnabled && task.billing ? (
                                <div>
                                    <dt>{t("library.tasks.detail.billing")}</dt>
                                    <dd>
                                        <TaskBilling billing={task.billing} />
                                    </dd>
                                </div>
                            ) : null}
                        </dl>
                    </section>

                    <section className="task-detail-section">
                        <h3>{t("library.tasks.params.title")}</h3>
                        {loading && !task.inputJson ? (
                            <div className="task-detail-empty">{t("library.tasks.detail.loading")}</div>
                        ) : parameters.length ? (
                            <dl className="task-detail-facts">
                                {parameters.map((field) => (
                                    <div key={field.label}>
                                        <dt>{field.label}</dt>
                                        <dd className="is-wrap">{field.value}</dd>
                                    </div>
                                ))}
                            </dl>
                        ) : (
                            <div className="task-detail-empty">{t("library.tasks.params.empty")}</div>
                        )}
                    </section>

                    <details className="task-detail-raw">
                        <summary>{t("library.tasks.detail.result")}</summary>
                        <pre>{loading && !task.resultJson ? t("library.tasks.detail.loading") : formatTaskJson(task.resultJson)}</pre>
                    </details>
                </aside>
            </div>

            <footer className="task-detail-footer">
                <button type="button" className="task-detail-id" title={t("library.tasks.detail.taskId")} onClick={() => onCopy(task.id)}>
                    <span>{t("library.tasks.detail.taskId")}</span>
                    <code>{task.id}</code>
                    <Copy aria-hidden="true" />
                </button>
                <div className="task-detail-actions">
                    {failed ? (
                        <Button icon={<Bug className="size-4" />} onClick={onExportDiagnostics}>
                            {t("library.tasks.detail.exportDiagnostics")}
                        </Button>
                    ) : null}
                    {canQueryProviderTask(task) ? (
                        <Button icon={<RefreshCw className="size-4" />} loading={acting} onClick={onQueryProvider}>
                            {t("library.tasks.detail.queryProvider")}
                        </Button>
                    ) : null}
                    {task.canRecoverMedia ? (
                        <Button type="primary" icon={<RefreshCw className="size-4" />} loading={acting} onClick={onRetry}>
                            {t("library.tasks.row.retrySave")}
                        </Button>
                    ) : failed && !retryBlocked ? (
                        <Button type="primary" icon={<RotateCcw className="size-4" />} loading={acting} onClick={onRetry}>
                            {t("library.tasks.row.retryTask")}
                        </Button>
                    ) : null}
                    <Button onClick={onClose}>{t("common.close")}</Button>
                </div>
            </footer>
        </div>
    );
}

/** 结果画廊：大图 + 缩略图切换；图片可再放大预览，视频直接带控件播放。 */
function TaskResultGallery({ items, title, onPreview }: { items: TaskResultMedia[]; title: string; onPreview: (media: { url: string; kind: "image" | "video"; title: string }) => void }) {
    const { t } = useTranslation();
    const { message } = App.useApp();
    const [index, setIndex] = useState(0);
    const current = items[Math.min(index, items.length - 1)];
    const currentIsVideo = current.kind === "video";
    const openOriginal = async () => {
        const scope = getActiveUserScope();
        try {
            const url = await taskResultMediaAccess(current, "original", getResourceAccess);
            if (scope === getActiveUserScope()) onPreview({ url: resolveResourceAccessURL(url), kind: current.kind, title });
        } catch {
            message.error(t("library.tasks.row.previewUnavailable"));
        }
    };
    return (
        <div className="task-detail-gallery">
            <div className="task-detail-stage">
                <TaskResultPreview key={current.key} item={current} variant={currentIsVideo ? "playback" : "preview"} alt={t("library.tasks.detail.generatedItem", { index: index + 1 })} controls={currentIsVideo} />
                {!currentIsVideo ? (
                    <button type="button" className="task-detail-zoom" aria-label={t("library.tasks.row.previewImage")} title={t("library.tasks.row.previewImage")} onClick={() => void openOriginal()}>
                        <Maximize2 aria-hidden="true" />
                    </button>
                ) : null}
            </div>
            {items.length > 1 ? (
                <div className="task-detail-thumbs" role="group" aria-label={t("library.tasks.detail.generated")}>
                    {items.map((item, itemIndex) => (
                        <button key={item.key} type="button" aria-pressed={itemIndex === index} aria-label={t("library.tasks.detail.generatedItem", { index: itemIndex + 1 })} onClick={() => setIndex(itemIndex)}>
                            <TaskResultPreview item={item} variant="thumbnail" alt={t("library.tasks.detail.generatedItem", { index: itemIndex + 1 })} />
                        </button>
                    ))}
                </div>
            ) : null}
        </div>
    );
}

function TaskResultPreview({ item, variant, alt, controls = false }: { item: TaskResultMedia; variant: ResourceAccessVariant; alt: string; controls?: boolean }) {
    const { t } = useTranslation();
    const scope = getActiveUserScope();
    const identity = `${scope}:${item.key}:${variant}`;
    const [resolved, setResolved] = useState<{ identity: string; url: string; failed?: boolean }>();
    const { key, resourceId, url, kind } = item;
    useEffect(() => {
        let cancelled = false;
        void taskResultMediaAccess({ key, resourceId, url, kind }, variant, getResourceAccess)
            .then((value) => {
                if (!cancelled && scope === getActiveUserScope()) setResolved({ identity, url: resolveResourceAccessURL(value) });
            })
            .catch(() => {
                if (!cancelled && scope === getActiveUserScope()) setResolved({ identity, url: "", failed: true });
            });
        return () => {
            cancelled = true;
        };
    }, [identity, key, resourceId, url, kind, variant, scope]);
    if (resolved?.identity !== identity) return <div className="task-detail-empty">{t("library.tasks.detail.loading")}</div>;
    if (resolved.failed) return <div className="task-detail-stage-unavailable">{t("library.tasks.row.previewUnavailable")}</div>;
    return <MediaPreview src={resolved.url} kind={variant === "thumbnail" && resourceId ? "image" : kind} alt={alt} controls={controls} fallbackClassName="task-detail-stage-unavailable" />;
}

function canQueryProviderTask(task: GenerationTask) {
    return task.status === "failed" && !task.mediaStage && (task.type.startsWith("canvas_video") || task.type.startsWith("video_")) && Boolean(task.providerRequestId);
}

function formatDate(value?: string) {
    if (!value) return "-";
    const date = new Date(value);
    return Number.isNaN(date.getTime()) ? "-" : date.toLocaleString(currentLocale());
}

function formatLogTime(value: string) {
    const date = new Date(value);
    return Number.isNaN(date.getTime()) ? "-" : date.toLocaleString(currentLocale(), { month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", second: "2-digit" });
}

function formatTaskDuration(task: GenerationTask) {
    if (!task.createdAt) return "-";
    const start = new Date(task.startedAt || task.createdAt).getTime();
    const end = task.completedAt ? new Date(task.completedAt).getTime() : isTaskActive(task) ? Date.now() : Number.NaN;
    if (!Number.isFinite(start) || !Number.isFinite(end) || end < start) return "-";
    const totalSeconds = Math.max(0, Math.floor((end - start) / 1000));
    const minutes = Math.floor(totalSeconds / 60);
    const seconds = totalSeconds % 60;
    return minutes ? translate("library.tasks.detail.durationMinutes", { minutes, seconds }) : translate("library.tasks.detail.durationSeconds", { seconds });
}

function taskParameterFields(inputJson?: string) {
    const input = parseTaskInput(inputJson);
    if (!input) return [];
    const config = asRecord(input.config);
    const fields: Array<{ label: string; value: string }> = [];
    const add = (label: string, value: unknown) => {
        const text = formatParameterValue(value);
        if (text) fields.push({ label, value: text });
    };

    add(translate("library.tasks.params.mode"), input.mode);
    add(translate("library.tasks.params.size"), config.size);
    add(translate("library.tasks.params.resolution"), config.vquality || config.quality);
    add(translate("library.tasks.params.duration"), config.videoSeconds === undefined ? undefined : translate("library.tasks.params.seconds", { seconds: config.videoSeconds }));
    add(translate("library.tasks.params.count"), config.count);
    add(translate("library.tasks.params.audio"), booleanParameter(config.videoGenerateAudio));
    add(translate("library.tasks.params.watermark"), booleanParameter(config.videoWatermark));
    add(translate("library.tasks.params.voice"), config.audioVoice);
    add(translate("library.tasks.params.audioFormat"), config.audioFormat);
    add(translate("library.tasks.params.audioSpeed"), config.audioSpeed);

    // 参考素材只在确实有输入时列出，避免一串「无」占满参数区。
    add(translate("library.tasks.params.referenceImages"), formatReferenceList(input.referenceImages, translate("library.tasks.params.imageItem")));
    add(translate("library.tasks.params.referenceVideos"), formatReferenceList(input.referenceVideos, translate("library.tasks.params.videoItem")));
    add(translate("library.tasks.params.referenceAudios"), formatReferenceList(input.referenceAudios, translate("library.tasks.params.audioItem")));
    add(translate("library.tasks.params.mask"), formatReferenceList(input.mask ? [input.mask] : [], translate("library.tasks.params.maskItem")));
    return fields;
}

function parseTaskInput(value?: string): Record<string, unknown> | null {
    if (!value) return null;
    try {
        const parsed: unknown = JSON.parse(value);
        return asRecord(parsed);
    } catch {
        return null;
    }
}

function asRecord(value: unknown): Record<string, unknown> {
    return value && typeof value === "object" && !Array.isArray(value) ? (value as Record<string, unknown>) : {};
}

function formatParameterValue(value: unknown) {
    if (value === undefined || value === null || value === "") return "";
    if (typeof value === "string") return value;
    if (typeof value === "number" || typeof value === "boolean") return String(value);
    return "";
}

function booleanParameter(value: unknown) {
    if (value === true || value === "true") return translate("library.tasks.params.yes");
    if (value === false || value === "false") return translate("library.tasks.params.no");
    return undefined;
}

function formatReferenceList(value: unknown, kind: string) {
    if (!Array.isArray(value) || !value.length) return undefined;
    return value
        .map((item, index) => {
            const reference = asRecord(item);
            const name = typeof reference.name === "string" && reference.name.trim() && !/^https?:|^data:|^blob:/i.test(reference.name) ? reference.name.trim() : translate("library.tasks.params.numberedItem", { kind, index: index + 1 });
            const dimensions = typeof reference.width === "number" && typeof reference.height === "number" ? `${reference.width}×${reference.height}` : "";
            const duration = typeof reference.durationMs === "number" && reference.durationMs > 0 ? `${Math.round(reference.durationMs / 100) / 10}s` : "";
            const details = [dimensions, duration].filter(Boolean).join(translate("library.tasks.params.detailSeparator"));
            return details ? translate("library.tasks.params.itemWithDetails", { name, details }) : name;
        })
        .join(translate("library.tasks.params.listSeparator"));
}

function formatTaskJson(value?: string) {
    if (!value) return translate("library.tasks.detail.none");
    try {
        return JSON.stringify(JSON.parse(value), null, 2);
    } catch {
        return value;
    }
}
