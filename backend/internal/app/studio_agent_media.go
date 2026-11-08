package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"yingce/backend/internal/model"
	"yingce/backend/internal/platformintegration"
	"yingce/backend/internal/repository"
)

type StudioAgentMediaQuote struct {
	ID              string         `json:"id"`
	WalletMaxPoints string         `json:"walletMaxPoints"`
	PriceVersion    string         `json:"priceVersion"`
	ExpiresAt       time.Time      `json:"expiresAt"`
	Parameters      map[string]any `json:"parameters"`
}

type studioAgentMediaPrepared struct {
	Envelope       studioExecutionEnvelope `json:"envelope"`
	Request        CreateTaskRequest       `json:"request"`
	Plan           cloudAgentMediaPlan     `json:"plan"`
	DependencyHash string                  `json:"dependencyHash"`
	Quote          StudioAgentMediaQuote   `json:"quote"`
}

func (s *Service) studioAgentMediaSelection(userID, runID, name string) (platformintegration.ModelSelection, error) {
	root, err := s.repo.StudioExecution(userID, runID)
	if err != nil {
		return platformintegration.ModelSelection{}, err
	}
	var frozen studioExecutionEnvelope
	if err := s.decryptStudioValue(root.EnvelopeEncrypted, &frozen); err != nil {
		return platformintegration.ModelSelection{}, err
	}
	for _, item := range frozen.Catalog {
		if item.ModelName == name {
			selected, err := platformintegration.SelectModel(item, "")
			if err != nil {
				return selected, BadAuthRequest(err.Error())
			}
			return selected, nil
		}
	}
	return platformintegration.ModelSelection{}, BadAuthRequest("模型不在本轮平台目录中，请重新开始会话")
}

func (s *Service) resolveStudioAgentMediaOptions(userID string, a *cloudAgentMediaArgs, state *cloudAgentRuntime) error {
	if a.ChannelID != StudioPlatformChannelID || a.LogicalModelID != "" || a.Mode != "image" && a.Mode != "video" || a.VideoArkPrivateAssetUpload != nil {
		return BadAuthRequest("请选择平台图片或视频模型；供应商素材库暂不可用")
	}
	selected, err := s.studioAgentMediaSelection(userID, state.RuntimeRunID, a.ChannelModelKey)
	if err != nil {
		return err
	}
	if selected.Kind != a.Mode {
		return BadAuthRequest("模型与创作类型不匹配")
	}
	var schema struct {
		Properties map[string]map[string]any `json:"properties"`
	}
	if json.Unmarshal(selected.Schema, &schema) != nil {
		return NewAppError(502, "模型参数合同无效")
	}
	defaultValue := func(keys ...string) string {
		for _, key := range keys {
			if value := studioPublishedDefault(selected, key, schema.Properties[key]); value != nil {
				return fmt.Sprint(value)
			}
		}
		return ""
	}
	if a.Size == "" {
		a.Size = defaultValue("aspect_ratio", "ratio", "size")
	}
	if a.Mode == "image" {
		params := map[string]any{}
		for k, v := range selected.Defaults {
			params[studioParameterName(k)] = v
		}
		if a.ImageParameters != "" {
			var supplied map[string]any
			if json.Unmarshal([]byte(a.ImageParameters), &supplied) != nil {
				return BadAuthRequest("图片参数格式无效")
			}
			for k, v := range supplied {
				params[k] = v
			}
		}
		for k, v := range map[string]string{"size": a.Size, "quality": a.Quality, "imageResolution": a.ImageResolution} {
			if v != "" {
				params[k] = v
			}
		}
		raw, _ := json.Marshal(params)
		a.ImageParameters = string(raw)
	} else {
		if a.Duration == 0 {
			raw := defaultValue("duration", "seconds")
			if raw != "" {
				value, err := strconv.Atoi(raw)
				if err != nil || value < 0 {
					return BadAuthRequest("平台视频时长默认值无效")
				}
				a.Duration = value
			}
		}
		if a.Quality == "" {
			a.Quality = defaultValue("resolution")
		}
		profile := &VideoCapabilityConfig{}
		for _, value := range studioPublishedValues(schema.Properties["resolution"]) {
			if resolution, ok := value.(string); ok {
				profile.Resolutions = append(profile.Resolutions, resolution)
			}
		}
		// Agent settings can retain the UI's 720 spelling; only restore a
		// matching published choice. Unsupported values still fail admission.
		if resolution := videoResolutionNameRequest(profile, a.Quality); resolution != "" {
			a.Quality = resolution
		}
	}
	return nil
}

func (s *Service) requestStudioAgentMediaApproval(run *model.CloudAgentExecution, state *cloudAgentRuntime, call cloudAgentCall) error {
	req, plan, err := s.prepareCloudAgentMedia(run, state, cloudAgentMediaCall(call))
	if err != nil {
		return s.cloudAgentMediaError(run, state, "admission", false, false, err)
	}
	selected, err := s.studioAgentMediaSelection(run.UserID, run.ID, req.Model)
	if err != nil {
		return err
	}
	if _, err := s.studioMediaBody(context.Background(), run.UserID, req, selected); err != nil {
		return s.cloudAgentMediaError(run, state, "admission", false, false, err)
	}
	policy, err := s.RuntimePolicy()
	if err != nil {
		return err
	}
	s.storageMu.Lock()
	defer s.storageMu.Unlock()
	return s.repo.MutateCloudAgent(run.UserID, run.ID, run.Revision, func(current *model.CloudAgentExecution, repo *repository.Repository) error {
		if err := createCloudAgentMediaNode(repo, run.UserID, state.Request.CanvasID, plan, nil, policy, cloudAgentCanvasEventRecorder(run.ID, state)); err != nil {
			return err
		}
		canvas, err := repo.CanvasProjectForUser(run.UserID, state.Request.CanvasID)
		if err != nil {
			return err
		}
		doc, err := creationDocument(canvas.PayloadJSON)
		if err != nil {
			return err
		}
		plan.Args.SnapshotHash = cloudAgentMediaContentHash(doc)
		raw, _ := json.Marshal(plan.Args)
		call.Function.Arguments = string(raw)
		state.Calls[state.CallIndex] = call
		preview := cloudAgentMediaApprovalPreview(plan, selected.Name)
		state.Approval = &cloudAgentApproval{ID: fmt.Sprintf("%s-%d-%d", run.ID, state.Step, state.CallIndex), Call: call, CallHash: cloudAgentApprovalCallHash(call), Preview: preview, ModelName: selected.Name, StudioMedia: true}
		state.PiResumePrompt = ""
		current.Status = "waiting_approval"
		state.event(run.ID, "approval_requested", map[string]any{"approvalId": state.Approval.ID, "toolName": call.Function.Name, "modelName": selected.Name, "arguments": json.RawMessage(call.Function.Arguments), "preview": preview, "studioMedia": true, "text": preview.Description})
		return cloudAgentSave(current, state)
	})
}

func (s *Service) QuoteStudioAgentMedia(ctx context.Context, userID string, session platformclient.SessionContext, runID, approvalID string) (*StudioAgentMediaQuote, *platformclient.Response, error) {
	response := &platformclient.Response{Header: make(http.Header)}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if _, _, err := s.cloudAgentTask(userID, runID); err != nil {
		return nil, response, err
	}
	run, err := s.repo.CloudAgent(userID, runID)
	if err != nil {
		return nil, response, err
	}
	state, err := cloudAgentDecode(run)
	if err != nil {
		return nil, response, err
	}
	if run.Status != "waiting_approval" || state.Approval == nil || state.Approval.ID != approvalID || !state.Approval.StudioMedia || state.Approval.Decision != "" {
		return nil, response, creationConflict("媒体审批不存在或已经处理")
	}
	req, plan, err := s.prepareCloudAgentMedia(run, &state, state.Approval.Call)
	if err != nil {
		return nil, response, err
	}
	selected, err := s.studioSelectTaskModel(ctx, session, req, response)
	if err != nil {
		return nil, response, err
	}
	body, err := s.studioMediaBody(ctx, userID, req, selected)
	if err != nil {
		return nil, response, err
	}
	req.Platform = &StudioTaskOptions{VariantID: selected.ID, ExpectedPriceVersion: selected.Price.Version}
	preview, err := s.studioPreviewMedia(ctx, session, req, selected, body, response, false)
	if err != nil {
		return nil, response, err
	}
	req.Platform.WalletMaxPoints = preview.WalletPoints
	id := cloudAgentID(userID, runID+":media:"+approvalID)
	public, _ := json.Marshal(map[string]any{"mode": selected.Kind, "metadata": studioTaskMetadata(req.Input)})
	task := model.Task{ID: id, UserID: userID, ProjectID: state.Request.CanvasID, AgentRunID: runID, Type: req.Type, Operation: req.Operation, Provider: "platform", Model: selected.Name, Prompt: req.Prompt, InputJSON: string(public), Status: model.TaskStatusQueued, Stage: "等待已批准的平台媒体任务"}
	canvas, err := s.repo.CanvasProjectForUser(userID, state.Request.CanvasID)
	if err != nil {
		return nil, response, err
	}
	doc, err := creationDocument(canvas.PayloadJSON)
	if err != nil {
		return nil, response, err
	}
	dependency, err := cloudAgentMediaDependencyHash(doc, plan.Args)
	if err != nil {
		return nil, response, err
	}
	input, err := studioDecodeInput(req)
	if err != nil {
		return nil, response, err
	}
	parameters, err := studioMediaSpecifications(input, selected)
	if err != nil {
		return nil, response, err
	}
	quote := StudioAgentMediaQuote{ID: newID(), WalletMaxPoints: preview.WalletPoints, PriceVersion: preview.PriceVersion, ExpiresAt: time.Now().Add(cloudAgentMediaQuoteLifetime), Parameters: parameters}
	prepared := studioAgentMediaPrepared{Envelope: studioExecutionEnvelope{Task: task, Selection: selected, RequestID: "studio-" + id, AgentRunID: runID, ContentType: "application/json", Body: body, Consumption: studioMediaConsumption(preview, req.Platform)}, Request: req, Plan: *plan, DependencyHash: dependency, Quote: quote}
	encrypted, err := s.encryptStudioValue(prepared)
	if err != nil {
		return nil, response, err
	}
	err = s.repo.MutateCloudAgent(userID, runID, run.Revision, func(current *model.CloudAgentExecution, repo *repository.Repository) error {
		latest, err := repo.CanvasProjectForUser(userID, state.Request.CanvasID)
		if err != nil {
			return err
		}
		doc, err := creationDocument(latest.PayloadJSON)
		if err != nil {
			return err
		}
		now, err := cloudAgentMediaDependencyHash(doc, plan.Args)
		if err != nil {
			return err
		}
		if now != dependency {
			return creationConflict("画布内容已变化，请重新申请审批")
		}
		state.Approval.StudioQuote = &quote
		state.Approval.StudioPreparedEncrypted = encrypted
		state.event(runID, "approval_quoted", map[string]any{"approvalId": approvalID, "studioQuote": quote})
		return cloudAgentSave(current, &state)
	})
	return &quote, response, err
}

// The decision, frozen request, canvas binding and native Pi checkpoint commit
// together. A lost response can only recover this decision, never re-quote it.
func (s *Service) DecideStudioAgentMedia(userID, runID, approvalID, decision, reason, quoteID string, settings *CloudAgentMediaSettings) error {
	if decision != "approve" {
		return s.DecideCloudAgentApproval(userID, runID, approvalID, decision, reason, settings)
	}
	if settings != nil {
		return BadAuthRequest("修改参数后需要重新申请报价，当前审批只执行已展示的参数")
	}
	if len(reason) > 2000 {
		return BadAuthRequest("审批理由过长")
	}
	if _, _, err := s.cloudAgentTask(userID, runID); err != nil {
		return err
	}
	run, err := s.repo.CloudAgent(userID, runID)
	if err != nil {
		return err
	}
	state, err := cloudAgentDecode(run)
	if err != nil {
		return err
	}
	if previous, ok := state.Decisions[approvalID]; ok {
		if state.DecisionPreparedHashes[approvalID] == "" && quoteID == "" {
			return s.DecideCloudAgentApproval(userID, runID, approvalID, decision, reason, settings)
		}
		if previous == decision && quoteID != "" && state.DecisionPreparedHashes[approvalID] == quoteID {
			return nil
		}
		return cloudAgentDecisionConflict(previous)
	}
	if state.Approval == nil || !state.Approval.StudioMedia {
		return s.DecideCloudAgentApproval(userID, runID, approvalID, decision, reason, settings)
	}
	if run.Status != "waiting_approval" || state.Approval.ID != approvalID || state.Approval.StudioQuote == nil || quoteID == "" || state.Approval.StudioQuote.ID != quoteID || !time.Now().Before(state.Approval.StudioQuote.ExpiresAt) {
		return creationConflict("报价尚未确认或已过期，请刷新报价后再批准")
	}
	var prepared studioAgentMediaPrepared
	if err := s.decryptStudioValue(state.Approval.StudioPreparedEncrypted, &prepared); err != nil {
		return err
	}
	if prepared.Quote.ID != quoteID {
		return creationConflict("审批报价身份不匹配")
	}
	if !studioMediaConsumptionValid(prepared.Envelope.Consumption) {
		return creationConflict("原媒体消费版本未冻结，请重新确认报价")
	}
	body, err := s.studioMediaBody(context.Background(), userID, prepared.Request, prepared.Envelope.Selection)
	if err != nil {
		return err
	}
	if !bytes.Equal(body, prepared.Envelope.Body) {
		return creationConflict("参考素材已变化，请重新申请报价")
	}
	policy, err := s.RuntimePolicy()
	if err != nil {
		return err
	}
	frozen, err := s.encryptStudioValue(prepared.Envelope)
	if err != nil {
		return err
	}
	key := cloudAgentApprovalFollowUpKey(runID, approvalID)
	followUp := s.claimCloudAgentApprovalFollowUp(key)
	if followUp {
		defer s.releaseCloudAgentApprovalFollowUp(key)
	}
	s.storageMu.Lock()
	err = s.repo.MutateCloudAgent(userID, runID, run.Revision, func(current *model.CloudAgentExecution, repo *repository.Repository) error {
		if current.Status != "waiting_approval" || current.CleanupPending || !time.Now().Before(prepared.Quote.ExpiresAt) {
			return creationConflict("本轮已经停止")
		}
		root, err := repo.StudioExecution(userID, runID)
		if err != nil {
			return err
		}
		if root.State != "agent" || root.Released {
			return creationConflict("本轮授权已结束")
		}
		canvas, err := repo.CanvasProjectForUser(userID, state.Request.CanvasID)
		if err != nil {
			return err
		}
		doc, err := creationDocument(canvas.PayloadJSON)
		if err != nil {
			return err
		}
		dependency, err := cloudAgentMediaDependencyHash(doc, prepared.Plan.Args)
		if err != nil {
			return err
		}
		if dependency != prepared.DependencyHash {
			return creationConflict("画布内容已变化，请重新申请审批")
		}
		prepared.Plan.Args.SnapshotHash = cloudAgentMediaContentHash(doc)
		prepared.Plan.Args.DraftRunID = runID
		task := prepared.Envelope.Task
		if err := createCloudAgentMediaNode(repo, userID, state.Request.CanvasID, &prepared.Plan, &task, policy, cloudAgentCanvasEventRecorder(runID, &state)); err != nil {
			return err
		}
		execution, err := repo.CreateStudioExecution(model.StudioExecution{TaskID: task.ID, UserID: userID, CommandID: "agent-media:" + task.ID, IntentHash: quoteID, EnvelopeEncrypted: frozen, State: "authorizing", Released: true})
		if err != nil {
			return err
		}
		if execution.IntentHash != quoteID {
			return creationConflict("该媒体步骤已有其他请求")
		}
		if err := repo.QueueStudioExecution(task, root.AuthorizationEncrypted, policy.Task.ActiveTaskLimit); err != nil {
			return err
		}
		if state.Decisions == nil {
			state.Decisions = map[string]string{}
		}
		if state.DecisionPreparedHashes == nil {
			state.DecisionPreparedHashes = map[string]string{}
		}
		state.Decisions[approvalID] = decision
		state.DecisionPreparedHashes[approvalID] = quoteID
		state.Approval.Decision = decision
		state.Approval.Reason = reason
		state.TaskIDs = append(state.TaskIDs, task.ID)
		state.MediaTaskID = task.ID
		state.Generations++
		state.VideoSeconds += prepared.Plan.Args.Duration
		current.Status = "running"
		state.event(runID, "approval_decided", map[string]any{"approvalId": approvalID, "decision": decision, "studioQuote": prepared.Quote})
		state.event(runID, "generation_task_created", map[string]any{"toolName": "generate_media", "taskId": task.ID, "nodeId": prepared.Plan.Args.NodeID, "title": prepared.Plan.Args.Title, "mode": prepared.Plan.Args.Mode, "canvasId": state.Request.CanvasID, "text": "媒体任务已按确认的报价提交"})
		return cloudAgentSave(current, &state)
	})
	s.storageMu.Unlock()
	if err == nil {
		s.wakeTaskDispatcher()
	}
	if err == nil && followUp {
		s.startApprovedCloudAgentMediaWaiter(userID, runID, state.MediaTaskID)
	}
	return err
}
