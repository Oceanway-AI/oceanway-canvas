package app

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptrace"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"yingce/backend/internal/model"
	"yingce/backend/internal/platformintegration"
	"yingce/backend/internal/repository"
)

func (s *Service) createStudioGeneration(ctx context.Context, userID string, session platformclient.SessionContext, req CreateTaskRequest, selected platformintegration.ModelSelection, id, hash string, response *platformclient.Response) (*model.Task, *platformclient.Response, error) {
	// Studio projects and their Resource bindings are currently personal. A
	// browser-supplied space must never select a different payer.
	if req.Platform != nil && req.Platform.WorkspaceID != "" {
		return nil, response, Forbidden("当前项目未绑定该素材空间")
	}
	body, err := s.studioMediaBody(ctx, userID, req, selected)
	if err != nil {
		return nil, response, err
	}
	input, err := studioPublicTaskInput(selected.Kind, req)
	if err != nil {
		return nil, response, err
	}
	command := platformclient.MediaCommand{RequestID: req.CommandID, ProductTaskID: id, Model: selected.Name, VariantID: selected.ID, NativeRequest: body}
	if o := req.Platform; o != nil {
		command.WorkspaceID = o.WorkspaceID
		command.Consumption = &platformclient.MediaGenerationConstraints{ExpectedCapabilityRevision: o.ExpectedCapabilityRevision, ExpectedConnectionRevision: o.ExpectedConnectionRevision, ExpectedPriceVersion: o.ExpectedPriceVersion, WalletMaxPoints: o.WalletMaxPoints, BillingPreference: o.BillingPreference}
	}
	encoded, err := s.encryptStudioValue(command)
	if err != nil {
		return nil, response, err
	}
	now := time.Now()
	task := model.Task{ID: id, UserID: userID, ProjectID: req.ProjectID, Type: req.Type, Operation: req.Operation, Provider: "platform", Model: selected.Name, Prompt: req.Prompt, InputJSON: input, Status: model.TaskStatusRunning, Stage: "正在提交生成", StartedAt: &now, TraceID: req.TraceID, RequestID: req.RequestID, WorkbenchKind: workbenchKindFromTaskInput(req.Input, req.Type), PlatformGenerationRequest: command.RequestID, PlatformWorkspaceID: command.WorkspaceID}
	ex := model.StudioExecution{TaskID: id, UserID: userID, CommandID: req.CommandID, IntentHash: hash, State: "generation_v1", EnvelopeEncrypted: encoded}
	if task.ProjectID != "" {
		if winner, found, e := s.joinStudioGenerationApproval(ctx, req, &task, command, hash, response); found || e != nil {
			return winner, response, e
		}
	}
	first, err := s.repo.RegisterStudioGeneration(ex, &task)
	if errors.Is(err, repository.ErrTaskStateConflict) {
		return nil, response, NewAppError(409, "此任务命令已用于不同的创作意图")
	}
	if err != nil {
		return nil, response, err
	}
	if first {
		auth := platformclient.MediaGenerationAuth{Session: &session, WorkspaceID: command.WorkspaceID}
		if err = s.sendStudioGeneration(ctx, &task, auth, command, response); err != nil {
			return nil, response, err
		}
	}
	out, err := s.Task(userID, id)
	return out, response, err
}

type studioGenerationSlot struct {
	Index         int            `json:"index"`
	Kind          string         `json:"kind"`
	ContentState  string         `json:"contentState"`
	PreviewState  string         `json:"previewState"`
	MaterialState string         `json:"materialState"`
	FailureCode   string         `json:"failureCode,omitempty"`
	Recoverable   bool           `json:"recoverable,omitempty"`
	PreviewURL    string         `json:"previewUrl,omitempty"`
	ResourceID    string         `json:"resourceId,omitempty"`
	Media         map[string]any `json:"media,omitempty"`
}

func studioGenerationAsset(o platformclient.MediaGenerationOutput) platformclient.Asset {
	return platformclient.Asset{AssetID: o.AssetID, Owner: *o.Owner, Kind: o.Kind, State: "ready", Version: platformclient.AssetVersion{AssetID: o.AssetID, VersionID: o.VersionID, FileID: o.FileID}, File: platformclient.AssetFile{FileID: o.FileID, State: "ready", SHA256: o.SHA256, ByteLength: strconv.FormatInt(o.Bytes, 10), ContentType: o.MIMEType, Width: o.Width, Height: o.Height, DurationMs: o.DurationMS}}
}

func (s *Service) projectStudioGeneration(ctx context.Context, task *model.Task, auth platformclient.MediaGenerationAuth, g platformclient.MediaGeneration, response *platformclient.Response) error {
	if g.RequestID != task.PlatformGenerationRequest || g.Revision < task.PlatformRevision || (g.Revision == task.PlatformRevision && !task.PlatformProjectionPending) {
		return nil
	}
	if task.ProviderRequestID != "" && task.ProviderRequestID != g.TaskID {
		return NewAppError(502, "原任务标识不一致")
	}
	kind := "image"
	if task.Type == "canvas_video" {
		kind = "video"
	}
	call := &studioMaterialCall{s: s, ctx: ctx, userID: task.UserID, response: response}
	if auth.Execution != nil {
		call.m = s.studioPlatform.Client.WorkerMaterials(platformclient.MaterialContext{ExecutionAuthorization: auth.Execution.Authorization})
	} else {
		call.m = s.studioPlatform.Client.CurrentMaterials(*auth.Session)
	}
	settled := g.FundsState == "settled" || g.FundsState == "not_applicable"
	outputs := make([]studioGenerationSlot, 0, len(g.Outputs))
	media := []map[string]any{}
	pending := false
	unfinished := len(g.Outputs) == 0
	failed := 0
	for _, o := range g.Outputs {
		if o.Kind != kind {
			return NewAppError(502, "平台输出类型不匹配")
		}
		slot := studioGenerationSlot{Index: o.Index, Kind: o.Kind, ContentState: o.ContentState, PreviewState: o.PreviewState, MaterialState: o.MaterialState}
		if o.Error != nil {
			slot.FailureCode = o.Error.Code
			slot.Recoverable = o.Error.Recovery.Action == "query_original" && o.Error.Recovery.Availability == "available"
		}
		if settled && o.PreviewState == "available" && o.PreviewPath != "" {
			slot.PreviewURL = "/api/tasks/" + task.ID + "/previews/" + strconv.Itoa(o.Index)
		}
		switch o.MaterialState {
		case "saved":
			if !settled || o.Owner == nil {
				pending = true
				break
			}
			asset := studioGenerationAsset(o)
			var resource *model.Resource
			var err error
			if auth.Session != nil && auth.Session.Origin == "" {
				// A same-origin browser GET may omit Origin. Only use an already
				// authorized local binding until the browser sends an adoption POST.
				repo := s.repo.WithContext(ctx)
				id := studioMaterialResourceID(task.UserID, asset)
				ref, findErr := repo.StudioMaterialReference(task.UserID, id)
				if findErr == nil && matchesStudioOriginal(*ref, asset) {
					resource, err = repo.ResourceForUser(task.UserID, id)
				}
			} else {
				resource, err = call.adopt(asset, StudioResourceMeta{})
			}
			if err != nil || resource == nil || resource.Status != model.ResourceStatusReady {
				pending = true
				break
			}
			slot.ResourceID = resource.ID
			item := map[string]any{"outputIndex": o.Index, "storageKey": "resource:" + resource.ID, "dataUrl": "/api/resources/" + resource.ID + "/file", "mimeType": resource.MimeType, "bytes": resource.Size}
			if o.Width > 0 && o.Height > 0 {
				item["width"], item["height"] = o.Width, o.Height
			}
			if o.DurationMS > 0 {
				item["durationMs"] = o.DurationMS
			}
			slot.Media = item
			media = append(media, item)
		case "failed":
			// The original status provides the observation window for recovery.
			if slot.Recoverable && g.RetryAfterMS > 0 {
				unfinished = true
			} else {
				failed++
			}
		case "deleted", "delivery_expired", "output_contract_violation":
			failed++
		default:
			unfinished = true
		}
		outputs = append(outputs, slot)
	}
	result := map[string]any{"mode": kind, "outputs": outputs, "executionState": g.ExecutionState, "fundsState": g.FundsState}
	if kind == "image" {
		result["images"] = media
	} else if len(media) > 0 {
		result["video"] = media[0]
	}
	if g.SettledPoints != "" {
		result["settledPoints"] = g.SettledPoints
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return err
	}
	task.ExecutionDiagnosticJSON = ""
	task.ResultJSON = string(encoded)
	task.PlatformRevision = g.Revision
	task.ProviderRequestID = g.TaskID
	task.PlatformRetryAfterMS = g.RetryAfterMS
	task.PlatformProjectionPending = pending
	task.Stage = "生成中"
	task.Status = model.TaskStatusRunning
	if g.Progress != nil {
		task.Progress = *g.Progress
	}
	if g.FundsState == "reserved" && g.RetryAfterMS > 0 && (g.ExecutionState == "succeeded" || g.ExecutionState == "failed" || g.ExecutionState == "cancelled" || g.ExecutionState == "rejected") {
		task.Stage = "正在处理原请求"
	} else if g.ExecutionState == "failed" || g.ExecutionState == "rejected" || g.ExecutionState == "cancelled" {
		task.Status = model.TaskStatusFailed
		task.Stage = "生成失败"
		task.Error, task.ExecutionDiagnosticJSON = studioMediaFailureDiagnostic(task.ID, g.Error, g.ExecutionState != "rejected")
	} else if g.ExecutionState == "succeeded" {
		task.Stage = "正在保存原件"
		if settled && !unfinished && !pending {
			if len(media) > 0 {
				task.Status = model.TaskStatusSucceeded
				task.Progress = 100
				task.Stage = "生成完成"
			} else if failed > 0 {
				task.Status = model.TaskStatusFailed
				task.Stage = "结果保存失败"
			}
		}
	}
	if task.Status != model.TaskStatusRunning {
		now := time.Now()
		task.CompletedAt = &now
		task.PlatformRetryAfterMS = 0
	}
	// The Platform receipt contains safe handles. Never persist response headers,
	// cookies or the one-use browser playback ticket.
	g.Response = nil
	raw, err := s.encryptStudioValue(g)
	if err != nil {
		return err
	}
	return s.repo.SaveStudioGenerationProjection(task, raw)
}

// An acknowledged final receipt contains all original file identities. Recover
// a lost project binding response against those files, even after a restart.
func (s *Service) storedStudioGenerationReceipt(ctx context.Context, task *model.Task) (platformclient.MediaGeneration, error) {
	var g platformclient.MediaGeneration
	ex, err := s.repo.WithContext(ctx).StudioExecution(task.UserID, task.ID)
	if err != nil {
		return g, err
	}
	if err = s.decryptStudioValue(ex.ResponseEncrypted, &g); err != nil {
		return g, err
	}
	if g.RequestID != task.PlatformGenerationRequest || g.Revision != task.PlatformRevision || g.TaskID != task.ProviderRequestID {
		return g, NewAppError(409, "原收据与当前任务不一致")
	}
	return g, nil
}

func (s *Service) StudioAdoptGenerationOutputs(ctx context.Context, task *model.Task, session platformclient.SessionContext) (*model.Task, *platformclient.Response, error) {
	response := &platformclient.Response{Header: make(http.Header)}
	if session.Origin == "" {
		return nil, response, Forbidden("采用素材需要当前页面的来源")
	}
	if task.PlatformGenerationRequest == "" {
		return nil, response, NotFound("任务不存在或不可访问")
	}
	if !task.PlatformProjectionPending {
		return task, response, nil
	}
	g, err := s.storedStudioGenerationReceipt(ctx, task)
	if err != nil {
		return nil, response, err
	}
	auth := platformclient.MediaGenerationAuth{Session: &session, WorkspaceID: task.PlatformWorkspaceID}
	if err = s.projectStudioGeneration(ctx, task, auth, g, response); err != nil {
		return nil, response, err
	}
	result, err := s.Task(task.UserID, task.ID)
	return result, response, err
}

func (s *Service) readStudioGenerationReceipt(ctx context.Context, task *model.Task, auth platformclient.MediaGenerationAuth) (platformclient.MediaGeneration, error) {
	if task.PlatformProjectionPending {
		g, err := s.storedStudioGenerationReceipt(ctx, task)
		if err != nil {
			return g, err
		}
		if g.ExecutionState == "succeeded" && (g.FundsState == "settled" || g.FundsState == "not_applicable") && len(g.Outputs) > 0 {
			final := true
			for _, o := range g.Outputs {
				switch o.MaterialState {
				case "saved", "deleted", "delivery_expired", "output_contract_violation":
				default:
					final = false
				}
			}
			if final {
				return g, nil
			}
		}
	}
	return s.studioPlatform.Client.GetMediaGeneration(ctx, auth, task.PlatformGenerationRequest)
}

func (s *Service) StudioReadGeneration(ctx context.Context, task *model.Task, session platformclient.SessionContext) (*model.Task, *platformclient.Response, error) {
	response := &platformclient.Response{Header: make(http.Header)}
	if task.PlatformGenerationRequest == "" || task.Status == model.TaskStatusSucceeded || task.Status == model.TaskStatusFailed {
		return task, response, nil
	}
	auth := platformclient.MediaGenerationAuth{Session: &session, WorkspaceID: task.PlatformWorkspaceID}
	g, err := s.readStudioGenerationReceipt(ctx, task, auth)
	g.Response.AppendCookies(response.Header)
	if err != nil {
		if rejected, e := s.studioGenerationRejection(task, err); rejected {
			if e != nil {
				return nil, response, e
			}
			final, e := s.Task(task.UserID, task.ID)
			return final, response, e
		}
		if g.Response != nil && (g.Response.StatusCode == 401 || g.Response.StatusCode == 403) {
			return nil, response, NotFound("任务不存在或不可访问")
		}
		return task, response, nil
	}
	if err = s.projectStudioGeneration(ctx, task, auth, g, response); err != nil {
		return nil, response, err
	}
	out, err := s.Task(task.UserID, task.ID)
	return out, response, err
}

func (s *Service) StudioGenerationPreview(ctx context.Context, userID, id string, index int, session platformclient.SessionContext) (string, *platformclient.Response, error) {
	task, err := s.repo.TaskForUser(userID, id)
	if err != nil || task.PlatformGenerationRequest == "" {
		return "", nil, NotFound("任务预览不存在")
	}
	auth := platformclient.MediaGenerationAuth{Session: &session, WorkspaceID: task.PlatformWorkspaceID}
	target, response, err := s.studioPlatform.Client.MediaGenerationPreview(ctx, auth, task.PlatformGenerationRequest, index)
	if err != nil {
		return "", response, studioMaterialError(err)
	}
	// The SDK validates the redirect target. The public Platform origin can
	// differ from its internal service origin.
	if path, ok := strings.CutPrefix(target, s.studioPlatform.Origin()+"/"); ok {
		target = s.studioPlatform.BrowserOrigin() + "/" + path
	}
	return target, response, nil
}

func studioGenerationPreviewSlots(task model.Task) []model.TaskPreviewOutput {
	if task.PlatformGenerationRequest == "" {
		return nil
	}
	var result struct{ Outputs []studioGenerationSlot }
	if json.Unmarshal([]byte(task.ResultJSON), &result) != nil {
		return nil
	}
	var slots []model.TaskPreviewOutput
	for _, o := range result.Outputs {
		if o.PreviewURL != "" {
			slots = append(slots, model.TaskPreviewOutput{Index: o.Index, Kind: o.Kind})
		}
	}
	return slots
}

func (s *Service) sendStudioGeneration(ctx context.Context, task *model.Task, auth platformclient.MediaGenerationAuth, command platformclient.MediaCommand, response *platformclient.Response) error {
	started := time.Now()
	var written, firstByte atomic.Int64
	ctx = httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
		WroteRequest: func(info httptrace.WroteRequestInfo) {
			if info.Err == nil {
				written.CompareAndSwap(0, time.Now().UnixMicro())
			}
		},
		GotFirstResponseByte: func() { firstByte.CompareAndSwap(0, time.Now().UnixMicro()) },
	})
	g, e := s.studioPlatform.Client.GenerateMedia(ctx, auth, command)
	// Record only the local correlation IDs and clocks. Missing transport events stay zero.
	slog.InfoContext(ctx, "studio media admission", "request_id", task.RequestID, "task_id", task.ID, "start_us", started.UnixMicro(), "write_us", written.Load(), "first_byte_us", firstByte.Load(), "received_us", time.Now().UnixMicro())
	g.Response.AppendCookies(response.Header)
	if e == nil {
		return s.projectStudioGeneration(ctx, task, auth, g, response)
	}
	if rejected, err := s.studioGenerationRejection(task, e); rejected {
		return err
	}
	return s.repo.MarkStudioGenerationUnknown(task, studioMediaSubmissionDiagnostic(g.Response, false, task.ID))
}

func (s *Service) studioGenerationRejection(task *model.Task, e error) (bool, error) {
	var failure *platformclient.MediaGenerationError
	if !errors.As(e, &failure) || !failure.RejectedBeforeDispatch || failure.Failure == nil {
		return false, nil
	}
	f := *failure.Failure
	messages := map[string]string{"insufficient_quota": "可用额度不足，请核对余额或已批准的额度。", "active_task_limit": "同时生成的任务已达上限。", "archive_capacity_insufficient": "素材空间容量不足。", "price_changed": "模型价格已更新，请刷新目录后确认。", "capability_changed": "模型能力已更新，请刷新目录后确认。", "invalid_request": "生成参数无效，请核对所选模型和输入。"}
	if message := messages[f.Code]; message != "" {
		f.Message = message
	}
	message, diagnostic := studioMediaFailureDiagnostic(task.ID, &f, false)
	return true, s.repo.RejectStudioGeneration(task, message, diagnostic)
}
