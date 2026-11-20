package app

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"yingce/backend/internal/model"
)

func studioFinalBill(task *model.Task, receipt platformclient.MediaGeneration) *model.StudioFinalWalletConsent {
	if task.Type != "canvas_video" || task.Provider != "platform" || task.PlatformWorkspaceID != "" ||
		task.PlatformGenerationRequest != receipt.RequestID || receipt.TaskID == "" || task.ProviderRequestID != receipt.TaskID ||
		receipt.OperationID == "" || receipt.ExecutionState != "succeeded" || receipt.FundsState != "reserved" || receipt.Response == nil {
		return nil
	}
	var body struct {
		Bill struct {
			State string `json:"state"`
			Total string `json:"total_points"`
		} `json:"final_wallet_consent"`
	}
	if json.Unmarshal(receipt.Response.Body, &body) != nil || (body.Bill.State != "required" && body.Bill.State != "recorded") || !studioValidDecimal(body.Bill.Total) {
		return nil
	}
	return &model.StudioFinalWalletConsent{State: body.Bill.State, TotalPoints: body.Bill.Total}
}

func (s *Service) StudioConfirmFinalWalletBill(ctx context.Context, userID, id, total string, session platformclient.SessionContext) (*platformclient.Response, error) {
	response := &platformclient.Response{Header: make(http.Header)}
	if session.Origin == "" || session.Cookie == "" {
		return response, Forbidden("确认结算需要原付款人的页面会话")
	}
	if !studioValidDecimal(total) {
		return response, BadAuthRequest("最终总额无效")
	}
	task, err := s.repo.WithContext(ctx).TaskForUser(userID, id)
	if err != nil || task.PlatformGenerationRequest == "" || task.Type != "canvas_video" || task.Provider != "platform" || task.PlatformWorkspaceID != "" {
		return response, NotFound("任务不存在或不可访问")
	}
	g, err := s.studioPlatform.Client.GetMediaGeneration(ctx, platformclient.MediaGenerationAuth{Session: &session}, task.PlatformGenerationRequest)
	g.Response.AppendCookies(response.Header)
	if err != nil {
		return response, NewAppError(503, "暂时无法读取原账单，请刷新任务")
	}
	bill := studioFinalBill(task, g)
	if bill == nil || bill.TotalPoints != total {
		return response, NewAppError(409, "原账单已更新，请刷新任务后核对")
	}
	if bill.State == "recorded" {
		return response, nil
	}
	receipt, err := s.studioPlatform.Client.ConfirmMediaGenerationFinalBill(ctx, session, g.RequestID, g.TaskID, g.OperationID, total)
	receipt.AppendCookies(response.Header)
	if err != nil {
		if receipt != nil && receipt.StatusCode == http.StatusConflict {
			return response, NewAppError(409, "原账单已更新，请刷新任务后核对")
		}
		return response, NewAppError(503, "暂未取得确认回执，请刷新原任务")
	}
	return response, nil
}
