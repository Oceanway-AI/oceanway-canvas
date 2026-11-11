package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"yingce/backend/internal/model"
)

func (s *Service) processStudioMedia(ctx context.Context, task *model.Task, execution *model.StudioExecution, envelope studioExecutionEnvelope, exec platformclient.ExecutionContext) error {
	// An old prepared request cannot invent revisions. Submitted requests must
	// still recover the original outcome, even when their old envelope lacks them.
	if execution.State == "prepared" && !studioMediaConsumptionValid(envelope.Consumption) {
		return s.failStudioTask(task, "原媒体消费版本未冻结，未提交生成请求")
	}
	var receipt platformclient.MediaRecovery
	if execution.ResponseEncrypted != "" {
		if err := s.decryptStudioValue(execution.ResponseEncrypted, &receipt); err != nil {
			return err
		}
		if !studioMediaReceiptMatches(receipt, envelope.RequestID, "") {
			return s.deferStudioTask(task)
		}
	} else {
		first, err := s.repo.SubmitStudioExecution(task)
		if err != nil {
			return err
		}
		if first {
			// Native synchronous responses can be large. Even a lost response is
			// recovered through the original receipt; no transient URL is adopted.
			submitted, submitErr := s.studioPlatform.Client.SubmitMedia(ctx, exec, platformclient.MediaRequest{Route: envelope.Selection.MediaRoute, RequestID: envelope.RequestID, ContentType: envelope.ContentType, ModelAction: envelope.Selection.Name + ":generateContent", Body: envelope.Body, Consumption: envelope.Consumption, PreferAsync: envelope.Selection.PreferAsync})
			rejection, rejected := studioMediaSubmissionRejection(submitted, envelope.RequestID)
			if submitErr != nil || rejected {
				diagnostic := studioMediaSubmissionDiagnostic(submitted, rejected, task.ID)
				if err := s.repo.UpdateTaskExecutionDiagnostic(task.UserID, task.ID, diagnostic); err != nil {
					return err
				}
			}
			if rejected {
				encrypted, err := s.encryptStudioValue(rejection)
				if err != nil {
					return err
				}
				if err := s.repo.SaveStudioExecutionResponse(task, encrypted); err != nil {
					return err
				}
				return s.failStudioMediaRejected(task)
			}
			return s.deferStudioTask(task)
		}
	}
	// A durable task ID is only the original recovery identity. UNKNOWN and
	// in-progress receipts must be refreshed after every poll or process restart.
	if receipt.Status != "SUCCESS" && receipt.Status != "FAILURE" {
		recovered, err := s.studioPlatform.Client.RecoverMedia(ctx, exec, envelope.RequestID)
		if err != nil {
			return s.deferStudioTask(task)
		}
		if !studioMediaReceiptMatches(recovered.Data, envelope.RequestID, receipt.TaskID) {
			return s.deferStudioTask(task)
		}
		receipt = recovered.Data
		if !studioMediaRejected(receipt, envelope.RequestID) && (receipt.TaskID == "" || !receipt.TaskAvailable && receipt.Status != "FAILURE") {
			return s.deferStudioTask(task)
		}
		encrypted, err := s.encryptStudioValue(receipt)
		if err != nil {
			return err
		}
		if err := s.repo.RefreshStudioExecutionResponse(task, execution.ResponseEncrypted, encrypted); err != nil {
			return err
		}
	}
	if studioMediaRejected(receipt, envelope.RequestID) {
		return s.failStudioMediaRejected(task)
	}
	if receipt.Status == "FAILURE" {
		return s.failStudioTask(task, "平台原任务生成失败；费用状态以平台原任务记录为准")
	}
	if receipt.Status != "SUCCESS" || !receipt.TaskAvailable || receipt.TaskID == "" {
		return s.deferStudioTask(task)
	}
	m := s.studioPlatform.Client.WorkerMaterials(platformclient.MaterialContext{ExecutionAuthorization: exec.Authorization})
	outputs, err := m.TaskOutputs(ctx, receipt.TaskID)
	if err != nil {
		return s.deferStudioTask(task)
	}
	state := outputs.Data
	if state.ExecutionState == "failed" {
		return s.failStudioTask(task, "平台原任务生成失败，请核对任务记录")
	}
	if state.MaterialState == "output_contract_violation" || state.MaterialState == "delivery_expired" {
		return s.failStudioTask(task, "平台原产物交付失败，请核对原任务")
	}
	if state.ExecutionState != "succeeded" || state.MaterialState != "saved" || len(state.Outputs) == 0 {
		return s.deferStudioTask(task)
	}
	sort.Slice(state.Outputs, func(i, j int) bool { return state.Outputs[i].Index < state.Outputs[j].Index })
	media := []any{}
	c := &studioMaterialCall{s: s, ctx: ctx, userID: task.UserID, m: m, response: &platformclient.Response{Header: make(http.Header)}}
	for _, output := range state.Outputs {
		if output.MaterialState != "saved" || output.Original == nil {
			return s.deferStudioTask(task)
		}
		original := output.Original
		if original.OperationID != receipt.TaskID || original.OutputIndex != output.Index {
			return s.deferStudioTask(task)
		}
		asset, err := m.Asset(ctx, original.Asset.AssetID, original.Asset.VersionID)
		if err != nil {
			return s.deferStudioTask(task)
		}
		a := asset.Data
		if a.AssetID != original.Asset.AssetID || a.Version.VersionID != original.Asset.VersionID || a.File.FileID != original.Asset.FileID || a.File.SHA256 != original.Asset.SHA256 || a.File.ByteLength != original.Asset.ByteLength || a.Owner != original.Owner || a.Kind != envelope.Selection.Kind {
			return s.failStudioTask(task, "平台原产物身份校验失败，请核对原任务")
		}
		resource, err := c.adopt(a, StudioResourceMeta{})
		if err != nil {
			return s.deferStudioTask(task)
		}
		media = append(media, map[string]any{"dataUrl": "/api/resources/" + resource.ID + "/file", "storageKey": "resource:" + resource.ID, "mimeType": resource.MimeType, "bytes": resource.Size})
	}
	result := map[string]any{"mode": envelope.Selection.Kind}
	if envelope.Selection.Kind == "image" {
		result["images"] = media
	} else {
		if len(media) != 1 {
			return s.failStudioTask(task, "平台视频产物数量不符合任务合同")
		}
		result["video"] = media[0]
	}
	return s.completeStudioTask(ctx, task, result)
}

func studioMediaReceiptMatches(receipt platformclient.MediaRecovery, requestID, taskID string) bool {
	return receipt.Version == "oceanway.media-request.v1" && receipt.RequestID == requestID && (taskID == "" || receipt.TaskID == taskID)
}

const studioMediaRejectedStage = "未执行生成"
const studioMediaRejectedMessage = "平台已拒绝原请求，未执行生成；请核对参数与报价"

func (s *Service) failStudioMediaRejected(task *model.Task) error {
	return s.failStudioTaskWithState(task, studioMediaRejectedStage, studioMediaRejectedMessage, studioMediaSubmissionDiagnostic(nil, true, task.ID))
}

// HTTP status does not prove that no paid request was sent. Only an explicit
// Platform receipt for this exact frozen request may end recovery as rejected.
func studioMediaRejected(receipt platformclient.MediaRecovery, requestID string) bool {
	return requestID != "" && receipt.Version == "oceanway.media-request.v1" && receipt.RequestID == requestID && receipt.Status == "FAILURE" && receipt.NoExecution
}

func studioMediaSubmissionRejection(response *platformclient.Response, requestID string) (platformclient.MediaRecovery, bool) {
	var result struct {
		Data platformclient.MediaRecovery `json:"data"`
	}
	if response == nil || json.Unmarshal(response.Body, &result) != nil || !studioMediaRejected(result.Data, requestID) {
		return platformclient.MediaRecovery{}, false
	}
	return result.Data, true
}

func studioMediaSubmissionDiagnostic(response *platformclient.Response, rejected bool, taskID string) string {
	code, outcome, retry := "media_submit_outcome_unknown", "unknown", "recover_original"
	message := "提交结果待核对，正在查询平台原请求，请勿重复生成"
	if response != nil && response.StatusCode >= 100 && response.StatusCode <= 599 {
		code = fmt.Sprintf("media_submit_http_%d", response.StatusCode)
		message = fmt.Sprintf("平台提交返回 HTTP %d，正在核对原请求，请勿重复生成", response.StatusCode)
	}
	if rejected {
		code, outcome, retry = "media_rejected_without_execution", "rejected", "review_request"
		message = studioMediaRejectedMessage
	}
	value, _ := json.Marshal(model.TaskExecutionDiagnostic{Code: code, Phase: "submission", SubmissionOutcome: outcome, RetryClass: retry, SafeMessage: message, DiagnosticID: taskID})
	return string(value)
}
