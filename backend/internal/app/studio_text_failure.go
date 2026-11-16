package app

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"yingce/backend/internal/model"
	"yingce/backend/internal/repository"
)

// Only recognized machine codes may cross from relay errors into user-visible tasks.
// Messages and unknown codes can contain provider URLs, prompts or credentials.
var studioTextRejectionMessages = map[string]string{
	"PLATFORM_MODEL_UNAVAILABLE": "所选模型当前不可用，请稍后再试或切换模型。",
	"model_not_found":            "当前渠道没有可用的所选模型，请检查模型选择。",
	"system_cpu_overloaded":      "平台当前繁忙，请稍后再试。",
	"system_memory_overloaded":   "平台当前繁忙，请稍后再试。",
	"system_disk_overloaded":     "平台当前繁忙，请稍后再试。",
	"overloaded_error":           "模型服务当前繁忙，请稍后再试。",
	"rate_limit_error":           "请求频率过高，请稍后再试。",
	"service_unavailable":        "平台服务暂时不可用，请稍后再试。",
	"server_error":               "模型服务暂时不可用，请稍后再试。",
	"invalid_request_error":      "模型请求参数无效，请检查输入。",
	"invalid_request":            "模型请求参数无效，请检查输入。",
	"authentication_error":       "平台未授权本次模型请求，请检查账号和模型权限。",
	"permission_denied":          "平台未授权本次模型请求，请检查账号和模型权限。",
	"insufficient_quota":         "本次模型请求的可用额度不足。",
	"content_policy_violation":   "模型服务拒绝了本次输入，请调整内容。",
}

func (s *Service) deferStudioTextRejection(task *model.Task, response *platformclient.Response) error {
	var rejection struct {
		Code  string `json:"code"`
		Error struct {
			Code string `json:"code"`
			Type string `json:"type"`
		} `json:"error"`
	}
	code := fmt.Sprintf("text_submit_http_%d", response.StatusCode)
	if json.Unmarshal(response.Body, &rejection) == nil {
		for _, candidate := range []string{rejection.Code, rejection.Error.Code, rejection.Error.Type} {
			if _, known := studioTextRejectionMessages[candidate]; known {
				code = candidate
				break
			}
		}
	}
	raw, _ := json.Marshal(model.TaskExecutionDiagnostic{Code: code, Phase: "submission", SubmissionOutcome: "unknown", SubmissionHTTPStatus: response.StatusCode, RetryClass: "recover_original", DiagnosticID: task.ID})
	return s.repo.Transaction(func(repo *repository.Repository) error {
		if err := repo.DeferRunningTaskForProviderPoll(task.ID, task.LeaseOwner, "正在查询平台原请求，请勿重复生成", 5*time.Second); err != nil {
			return err
		}
		return repo.UpdateTaskExecutionDiagnostic(task.UserID, task.ID, string(raw))
	})
}

func studioTextTemporaryRejection(code string, status int) bool {
	switch code {
	case "model_not_found", "invalid_request_error", "invalid_request", "authentication_error", "permission_denied", "insufficient_quota", "content_policy_violation":
		return false
	}
	if status == 0 {
		// Earlier Studio versions retained the HTTP status only for unclassified errors.
		if suffix, ok := strings.CutPrefix(code, "text_submit_http_"); ok {
			status, _ = strconv.Atoi(suffix)
		}
	}
	switch status {
	case 408, 429, 500, 502, 503, 504:
		return true
	case 200:
		switch code {
		case "rate_limit_error", "overloaded_error", "service_unavailable", "server_error", "system_cpu_overloaded", "system_memory_overloaded", "system_disk_overloaded":
			return true
		}
	}
	return false
}

func (s *Service) handleStudioTextFailure(task *model.Task, execution *model.StudioExecution, envelope studioExecutionEnvelope, receipt platformclient.TextRecovery) error {
	code := "text_request_failed"
	noExecution := receipt.Version == "oceanway.text-request.v1" && receipt.Status == "failed" && !receipt.Ready
	if submitted := taskExecutionDiagnostic(task); submitted != nil && submitted.Phase == "submission" && submitted.SubmissionOutcome == "unknown" && submitted.DiagnosticID == task.ID {
		code = submitted.Code
		agentStep := envelope.AgentRunID != "" && envelope.AgentRunID == task.AgentRunID && (task.Operation == cloudAgentStepOperation || task.Operation == cloudAgentContextCompactionOperation)
		if agentStep && noExecution && envelope.TextRetries >= 0 && envelope.TextRetries < cloudAgentModelStepRetries && studioTextTemporaryRejection(code, submitted.SubmissionHTTPStatus) {
			envelope.TextRetries++
			envelope.RequestID = fmt.Sprintf("studio-%s-retry-%d", task.ID, envelope.TextRetries)
			next, err := s.encryptStudioValue(envelope)
			if err != nil {
				return err
			}
			active, err := s.repo.PrepareStudioAgentTextRetry(task, execution.EnvelopeEncrypted, next, s.cloudAgentModelRetryDelay(envelope.TextRetries))
			if active || err != nil {
				return err
			}
		}
	}
	message := studioTextRejectionMessages[code]
	if message == "" {
		switch code {
		case "text_submit_http_429":
			message = "请求频率过高，请稍后再试。"
		case "text_submit_http_502", "text_submit_http_503", "text_submit_http_504":
			message = "平台服务暂时不可用，请稍后再试。"
		case "text_submit_http_401", "text_submit_http_403":
			message = "平台未授权本次模型请求，请检查账号和模型权限。"
		default:
			message = "平台未能执行这次模型请求，请检查模型选择或稍后再试。"
		}
	}
	outcome := "unknown"
	if noExecution {
		outcome = "rejected"
	}
	task.Error = message
	recordTaskDiagnostic(task, "submission", code, outcome, "review_request", nil)
	return s.failStudioTaskWithState(task, "模型请求失败", message, task.ExecutionDiagnosticJSON)
}

func (s *Service) failStudioTextResult(task *model.Task, message string) error {
	task.Error = message
	recordTaskDiagnostic(task, "result", "text_response_invalid", "submitted", "review_request", nil)
	return s.failStudioTaskWithState(task, "原任务待核对", message, task.ExecutionDiagnosticJSON)
}
