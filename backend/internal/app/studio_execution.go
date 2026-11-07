package app

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"yingce/backend/internal/model"
	"yingce/backend/internal/platformintegration"
	"yingce/backend/internal/repository"
)

type studioExecutionEnvelope struct {
	Task        model.Task                                  `json:"task"`
	Selection   platformintegration.ModelSelection          `json:"selection"`
	RequestID   string                                      `json:"requestId"`
	ContentType string                                      `json:"contentType"`
	Body        []byte                                      `json:"body"`
	Consumption *platformclient.MediaConsumptionConstraints `json:"consumption,omitempty"`
}

func studioTaskID(userID, commandID string) string {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("studio-task\n"+userID+"\n"+commandID)).String()
}

func (s *Service) StudioTaskByCommand(userID, commandID string) (*model.Task, error) {
	if !studioMaterialID.MatchString(commandID) {
		return nil, BadAuthRequest("任务命令标识无效")
	}
	return s.Task(userID, studioTaskID(userID, commandID))
}

func (s *Service) StudioCreateTask(ctx context.Context, userID string, session platformclient.SessionContext, req CreateTaskRequest) (*model.Task, *platformclient.Response, error) {
	response := &platformclient.Response{Header: make(http.Header)}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if !studioMaterialID.MatchString(req.CommandID) {
		return nil, response, BadAuthRequest("请提供稳定的任务命令标识")
	}
	if s.IsDraining() {
		return nil, response, NewAppError(503, "服务正在维护，请稍后查询原任务")
	}
	if strings.HasPrefix(req.Operation, "cloud_agent") && req.admission == nil {
		return nil, response, BadAuthRequest("Agent 任务必须通过 Agent 接口创建")
	}
	id := studioTaskID(userID, req.CommandID)
	intent, err := json.Marshal(req)
	if err != nil {
		return nil, response, BadAuthRequest("任务输入无效")
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(intent))
	existing, err := s.repo.StudioExecution(userID, id)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, response, err
	}
	if err == nil {
		if existing.IntentHash != hash {
			return nil, response, NewAppError(409, "此任务命令已用于不同的创作意图")
		}
		if existing.State != "authorizing" {
			task, e := s.Task(userID, id)
			return task, response, e
		}
	} else {
		if err := s.ensureTaskProjectActive(userID, req.ProjectID); err != nil {
			return nil, response, err
		}
		selected, e := s.studioSelectTaskModel(ctx, session, req, response)
		if e != nil {
			return nil, response, e
		}
		var body []byte
		var consumption *platformclient.MediaConsumptionConstraints
		if selected.Kind == "text" {
			body, e = studioTextBody(req, selected)
		} else {
			body, e = s.studioMediaBody(ctx, userID, req, selected)
			if e == nil {
				_, e = s.studioPreviewMedia(ctx, session, req, selected, body, response, true)
			}
			if e == nil {
				consumption = &platformclient.MediaConsumptionConstraints{ExpectedPriceVersion: req.Platform.ExpectedPriceVersion, WalletMaxPoints: req.Platform.WalletMaxPoints, BillingPreference: req.Platform.BillingPreference}
			}
		}
		if e != nil {
			return nil, response, e
		}
		publicInput, _ := json.Marshal(map[string]any{"mode": selected.Kind, "metadata": studioTaskMetadata(req.Input)})
		task := model.Task{ID: id, UserID: userID, ProjectID: req.ProjectID, Type: req.Type, Operation: req.Operation, Provider: "platform", Model: selected.Name, Prompt: req.Prompt, InputJSON: string(publicInput), Status: model.TaskStatusQueued, Stage: "等待平台任务调度", TraceID: req.TraceID, RequestID: req.RequestID, WorkbenchKind: workbenchKindFromTaskInput(req.Input, req.Type)}
		envelope := studioExecutionEnvelope{Task: task, Selection: selected, RequestID: "studio-" + id, ContentType: "application/json", Body: body, Consumption: consumption}
		encoded, e := s.encryptStudioValue(envelope)
		if e != nil {
			return nil, response, e
		}
		existing, e = s.repo.CreateStudioExecution(model.StudioExecution{TaskID: id, UserID: userID, CommandID: req.CommandID, IntentHash: hash, EnvelopeEncrypted: encoded, State: "authorizing"})
		if e != nil {
			return nil, response, e
		}
		if existing.IntentHash != hash {
			return nil, response, NewAppError(409, "此任务命令已用于不同的创作意图")
		}
	}
	var envelope studioExecutionEnvelope
	if err := s.decryptStudioValue(existing.EnvelopeEncrypted, &envelope); err != nil {
		return nil, response, err
	}
	grant, err := s.studioPlatform.Client.IssueExecution(ctx, session, id)
	grant.Response.AppendCookies(response.Header)
	if err != nil {
		return nil, response, WrapAppError(503, "执行授权尚未确认，请查询或继续原任务", err)
	}
	if grant.Data.AuthorizationID == "" || grant.Data.TaskReference != id || grant.Data.Product != "studio" || grant.Data.RevokedAt != 0 || grant.Data.ExpiresAt <= time.Now().Unix() {
		return nil, response, NewAppError(409, "原执行授权已失效，未提交生成请求")
	}
	auth, err := s.encryptSettingSecret(grant.Data.AuthorizationID)
	if err != nil {
		return nil, response, err
	}
	policy, err := s.RuntimePolicy()
	if err != nil {
		return nil, response, err
	}
	if err := s.repo.QueueStudioExecution(envelope.Task, auth, policy.Task.ActiveTaskLimit); err != nil {
		if errors.Is(err, repository.ErrActiveTaskLimit) {
			return nil, response, NewAppError(429, "正在执行的任务过多，请稍后继续原任务")
		}
		return nil, response, err
	}
	s.wakeTaskDispatcher()
	task, err := s.Task(userID, id)
	return task, response, err
}

func studioTaskMetadata(input map[string]any) map[string]any {
	source, _ := input["metadata"].(map[string]any)
	metadata := map[string]any{}
	for _, key := range []string{"clientOperationId", "nodeId", "conversationId", "messageId", "batchId", "batchIndex", "batchCount", "source"} {
		if v, ok := source[key]; ok {
			metadata[key] = v
		}
	}
	return metadata
}

func studioTextBody(req CreateTaskRequest, selected platformintegration.ModelSelection) ([]byte, error) {
	raw, err := json.Marshal(req.Input)
	if err != nil {
		return nil, BadAuthRequest("任务输入无效")
	}
	var input canvasGenerationInput
	if json.Unmarshal(raw, &input) != nil {
		return nil, BadAuthRequest("任务输入无效")
	}
	cfg := input.Config
	if cfg.APIKey != "" || cfg.SecretKey != "" || cfg.BaseURL != "" || len(cfg.Headers) > 0 || cfg.WorkflowID != "" || cfg.RunningHubWalletKey != "" || cfg.RunningHubUploadKey != "" || req.Provider != "" && req.Provider != "platform" || cfg.ChannelID != StudioPlatformChannelID || req.LogicalModelID != "" {
		return nil, BadAuthRequest("Studio 仅支持已发布的平台模型")
	}
	if len(input.ReferenceImages)+len(input.ReferenceVideos)+len(input.ReferenceAudios) > 0 || input.Mask != nil {
		return nil, NewAppError(501, "文本素材引用尚未接通")
	}
	canonical := canonicalAgentRequest{Messages: validatedTextHistory(input.TextHistory), SystemPrompt: cfg.SystemPrompt, ToolChoice: "auto"}
	canonical.Messages = append(canonical.Messages, map[string]any{"role": "user", "content": req.Prompt})
	if input.AgentRequests != nil {
		if input.AgentRequests.Canonical == nil {
			return nil, BadAuthRequest("Agent 必须提交标准会话意图")
		}
		canonical = *input.AgentRequests.Canonical
		canonical.PassReasoning = true
	}
	// Image references are adapted separately; arbitrary external URLs never
	// enter native bodies through canonical conversation content.
	for _, message := range canonical.Messages {
		if content, ok := message["content"].([]any); ok {
			for _, value := range content {
				part, _ := value.(map[string]any)
				if stringValue(part["type"]) != "text" {
					return nil, NewAppError(501, "文本素材引用尚未接通")
				}
			}
		}
	}
	cfg = providerConfig{Model: selected.Name, InterfaceType: studioOACProtocol(selected)}
	bodies, err := expandCanonicalAgentRequest(&canonical, cfg, false)
	if err != nil {
		return nil, BadAuthRequest("文本会话或工具历史无效")
	}
	body := bodies.ChatCompletion
	limitField := "max_tokens"
	switch selected.TextProtocol {
	case platformclient.Responses:
		body = bodies.Responses
		limitField = "max_output_tokens"
	case platformclient.Messages:
		body = bodies.Claude
	}
	limit := input.TextOptions.MaxOutputTokens
	if limit == 0 {
		limit = 4096
	}
	if limit < 1 || limit > 131072 {
		return nil, BadAuthRequest("文本输出上限无效")
	}
	body["model"], body["stream"], body[limitField] = selected.Name, false, limit
	return json.Marshal(body)
}

func (s *Service) encryptStudioValue(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return s.encryptSettingSecret(string(encoded))
}

func (s *Service) decryptStudioValue(encrypted string, target any) error {
	if !strings.HasPrefix(encrypted, encryptedSettingPrefix) {
		return errors.New("平台执行记录密文无效")
	}
	plain, err := s.decryptSettingSecret(encrypted)
	if err != nil {
		return errors.New("平台执行记录无法解密")
	}
	if err := json.Unmarshal([]byte(plain), target); err != nil {
		return errors.New("平台执行记录无效")
	}
	return nil
}
