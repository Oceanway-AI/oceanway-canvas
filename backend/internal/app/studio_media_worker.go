package app

import (
	"context"
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
	} else {
		first, err := s.repo.SubmitStudioExecution(task)
		if err != nil {
			return err
		}
		if first {
			// Native synchronous responses can be large. Even a lost response is
			// recovered through the original receipt; no transient URL is adopted.
			_, _ = s.studioPlatform.Client.SubmitMedia(ctx, exec, platformclient.MediaRequest{Route: envelope.Selection.MediaRoute, RequestID: envelope.RequestID, ContentType: envelope.ContentType, ModelAction: envelope.Selection.Name + ":generateContent", Body: envelope.Body, Consumption: envelope.Consumption})
			return s.deferStudioTask(task)
		}
		recovered, err := s.studioPlatform.Client.RecoverMedia(ctx, exec, envelope.RequestID)
		if err != nil {
			return s.deferStudioTask(task)
		}
		receipt = recovered.Data
		if receipt.RequestID != envelope.RequestID {
			return s.deferStudioTask(task)
		}
		if receipt.Status == "FAILURE" && receipt.NoExecution {
			return s.failStudioTask(task, "平台拒绝了原生成请求，请刷新参数与报价")
		}
		if !receipt.TaskAvailable || receipt.TaskID == "" {
			return s.deferStudioTask(task)
		}
		encrypted, err := s.encryptStudioValue(receipt)
		if err != nil {
			return err
		}
		if err := s.repo.SaveStudioExecutionResponse(task, encrypted); err != nil {
			return err
		}
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
