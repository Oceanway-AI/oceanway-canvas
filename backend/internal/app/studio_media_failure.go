package app

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"yingce/backend/internal/model"
	"yingce/backend/internal/repository"
)

// Platform's structured error describes the original request. It never grants
// authority to regenerate, change the budget, or infer a refund.
func studioMediaFailureDiagnostic(taskID string, failure *platformclient.MediaFailure, submitted bool) (string, string) {
	if failure == nil {
		failure = &platformclient.MediaFailure{Code: "media_failure_unknown", Category: "unknown", Source: "unknown", Stage: "unknown", Message: "媒体请求失败，具体原因待核对。"}
	}
	label := map[string]string{"user": "用户请求", "model": "模型服务", "platform": "平台服务", "unknown": "原因待确认"}[failure.Category]
	if label == "" {
		label = "原因待确认"
	}
	message := label + "：" + failure.Message
	outcome := "submitted"
	if !submitted {
		outcome = "rejected"
	}
	value, _ := json.Marshal(model.TaskExecutionDiagnostic{
		Code: failure.Code, Category: failure.Category, Source: failure.Source, Phase: failure.Stage,
		TaskSubmitted: submitted, SubmissionOutcome: outcome, RetryClass: "recover_original",
		RecoveryAction: failure.Recovery.Action, RecoveryAvailability: failure.Recovery.Availability,
		SafeMessage: message, DiagnosticID: taskID,
	})
	return message, string(value)
}

func (s *Service) failStudioMedia(task *model.Task, failure *platformclient.MediaFailure, submitted bool) error {
	message, diagnostic := studioMediaFailureDiagnostic(task.ID, failure, submitted)
	stage := "生成失败"
	if !submitted {
		stage = "未执行生成"
	}
	if failure != nil && (failure.Stage == "archive" || failure.Stage == "result_capture") {
		stage = "结果保存失败"
	}
	return s.failStudioTaskWithState(task, stage, message, diagnostic)
}

func (s *Service) deferStudioMediaArchive(task *model.Task, failure *platformclient.MaterialError) error {
	recovery := platformclient.MediaRecoveryAction{Action: "query_original", Availability: "available"}
	if failure.Recovery != nil {
		recovery = *failure.Recovery
	}
	message, diagnostic := studioMediaFailureDiagnostic(task.ID, &platformclient.MediaFailure{
		Code: failure.Code, Message: failure.Message, Category: failure.Category, Source: failure.Source, Stage: failure.Stage, Recovery: recovery,
	}, true)
	return s.repo.Transaction(func(repo *repository.Repository) error {
		if err := repo.DeferRunningTaskForProviderPoll(task.ID, task.LeaseOwner, "生成已完成，正在保存原结果。"+message, 5*time.Second); err != nil {
			return err
		}
		return repo.UpdateTaskExecutionDiagnostic(task.UserID, task.ID, diagnostic)
	})
}

func canRecoverTaskMedia(task model.Task) bool {
	if task.Provider != "platform" {
		return task.MediaRecoveryJSON != "" && task.Status == model.TaskStatusFailed
	}
	d := taskExecutionDiagnostic(&task)
	return task.Status == model.TaskStatusRunning && d != nil && d.Phase == "archive" && d.RecoveryAction == "recover_original_result" && d.RecoveryAvailability == "available"
}

// The existing product recovery button only wakes Platform's original archive.
// It does not reopen a failed generation or issue a fresh execution grant.
func (s *Service) recoverStudioMediaArchive(task *model.Task) (*model.Task, error) {
	if s.studioPlatform == nil || !canRecoverTaskMedia(*task) {
		return nil, BadAuthRequest("当前原任务没有可恢复的保存操作")
	}
	execution, err := s.repo.StudioExecution(task.UserID, task.ID)
	if err != nil {
		return nil, err
	}
	auth, err := s.decryptSettingSecret(execution.AuthorizationEncrypted)
	if err != nil {
		return nil, err
	}
	var receipt platformclient.MediaRecovery
	if err = s.decryptStudioValue(execution.ResponseEncrypted, &receipt); err != nil {
		return nil, err
	}
	if !studioMediaReceiptMatches(receipt, "studio-"+task.ID, "") || receipt.Status != "SUCCESS" || receipt.TaskID == "" {
		return nil, BadAuthRequest("原任务结果尚未确认")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	m := s.studioPlatform.Client.WorkerMaterials(platformclient.MaterialContext{ExecutionAuthorization: auth})
	state, err := m.TaskOutputs(ctx, receipt.TaskID)
	if err != nil {
		return nil, BadAuthRequest("暂时无法查询原结果保存状态")
	}
	if state.Data.Recovery == nil || state.Data.Recovery.Action != "recover_original_result" || state.Data.Recovery.Availability != "available" {
		return nil, BadAuthRequest("当前原结果不可手动恢复，请等待原任务更新")
	}
	if _, err = m.RecoverTaskOutputs(ctx, receipt.TaskID); err != nil {
		return nil, BadAuthRequest("原结果保存恢复未被接受，请刷新后查询")
	}
	return taskForOutput(*task), nil
}
