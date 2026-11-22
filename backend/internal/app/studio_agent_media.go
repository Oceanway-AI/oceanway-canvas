package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"maps"
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
	BillingMode     string         `json:"billingMode,omitempty"`
	EstimatedPoints string         `json:"estimatedPoints"`
	WalletMaxPoints string         `json:"walletMaxPoints,omitempty"` // Historical capped quotes.
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

// Writable turns share the same tool set. A pending action must be addressed
// by its ID so a mode change racing a new approval cannot consume that action.
func (s *Service) SetStudioAgentMode(userID, runID, from, mode string) error {
	if (mode != "auto" && mode != "request_approval") || (from != "auto" && from != "request_approval") {
		return BadAuthRequest("当前轮次只能在自动执行与请求审批之间切换")
	}
	run, err := s.repo.CloudAgent(userID, runID)
	if err != nil {
		return err
	}
	return s.repo.MutateCloudAgent(userID, runID, run.Revision, func(current *model.CloudAgentExecution, _ *repository.Repository) error {
		state, err := cloudAgentDecode(current)
		if err != nil {
			return err
		}
		if current.CleanupPending || current.Status != "running" && current.Status != "queued" || state.Request.ChannelID != StudioPlatformChannelID ||
			state.Request.PermissionMode == "read_only" || (state.Request.PermissionMode != from && state.Request.PermissionMode != mode) {
			return creationConflict("本轮状态已变化，请刷新后切换权限")
		}
		state.Request.PermissionMode = mode
		state.StudioAutoMedia = mode == "auto"
		return cloudAgentSave(current, &state)
	})
}

func (s *Service) studioAgentMediaSelection(userID, runID, name string, variant ...string) (platformintegration.ModelSelection, error) {
	id := ""
	if len(variant) > 0 {
		id = variant[0]
	}
	item, err := s.studioAgentMediaModel(userID, runID, name, id)
	if err != nil {
		return platformintegration.ModelSelection{}, err
	}
	selected, err := platformintegration.SelectModel(item, id)
	if err != nil {
		return selected, BadAuthRequest(err.Error())
	}
	return selected, nil
}

func (s *Service) studioAgentMediaModel(userID, runID, name, variant string) (platformclient.Model, error) {
	root, err := s.repo.StudioExecution(userID, runID)
	if err != nil {
		return platformclient.Model{}, err
	}
	var frozen studioExecutionEnvelope
	if err := s.decryptStudioValue(root.EnvelopeEncrypted, &frozen); err != nil {
		return platformclient.Model{}, err
	}
	if frozen.CatalogVersion != "" {
		detail, err := s.studioAgentCatalogDetail(userID, runID, name, variant)
		return detail.Model, err
	}
	for _, item := range frozen.Catalog {
		if item.ModelName == name {
			return item, nil
		}
	}
	return platformclient.Model{}, BadAuthRequest("模型不在本轮平台目录中，请重新开始会话")
}

func (s *Service) resolveStudioAgentMediaOptions(userID string, a *cloudAgentMediaArgs, state *cloudAgentRuntime, references map[string]any) error {
	if a.ChannelID != StudioPlatformChannelID || a.LogicalModelID != "" || a.Mode != "image" && a.Mode != "video" || a.VideoArkPrivateAssetUpload != nil {
		return BadAuthRequest("请选择平台图片或视频模型；供应商素材库暂不可用")
	}
	item, err := s.studioAgentMediaModel(userID, state.RuntimeRunID, a.ChannelModelKey, a.NativeVariantID)
	if err != nil {
		return err
	}
	profile, choices := studioNativeChoices(item)
	if a.NativeVariantID == "" && choices {
		input := maps.Clone(references)
		if input == nil {
			input = map[string]any{}
		}
		input["config"] = map[string]any{"channelId": StudioPlatformChannelID}
		input["metadata"] = map[string]any{"videoStartFrameNodeId": a.VideoStartFrameNodeID, "videoEndFrameNodeId": a.VideoEndFrameNodeID}
		decoded, err := studioDecodeInput(CreateTaskRequest{Input: input})
		if err != nil {
			return err
		}
		refs, err := s.studioNativeReferenceResources(context.Background(), userID, decoded)
		if err != nil {
			return err
		}
		return studioChooseAgentNativeVariant(a, state, item, profile, refs)
	}
	selected, err := platformintegration.SelectModel(item, a.NativeVariantID)
	if err != nil {
		return BadAuthRequest(err.Error())
	}
	return studioApplyAgentMediaOptions(a, state, selected)
}

func studioChooseAgentNativeVariant(a *cloudAgentMediaArgs, state *cloudAgentRuntime, item platformclient.Model, profile platformintegration.MediaProfile, refs []studioNativeReferenceResource) error {
	selected, err := studioChooseNativeVariant(item, profile, func(selected platformintegration.ModelSelection) ([]byte, error) {
		candidate := *a
		if err := studioApplyAgentMediaOptions(&candidate, state, selected); err != nil {
			return nil, err
		}
		var values map[string]any
		if err := json.Unmarshal([]byte(candidate.NativeParameters), &values); err != nil {
			return nil, BadAuthRequest("原生参数格式无效")
		}
		return studioNativeResolvedBody(a.Prompt, selected, values, refs)
	})
	if err != nil {
		return err
	}
	return studioApplyAgentMediaOptions(a, state, selected)
}

func studioApplyAgentMediaOptions(a *cloudAgentMediaArgs, state *cloudAgentRuntime, selected platformintegration.ModelSelection) error {
	if selected.Kind != a.Mode {
		return BadAuthRequest("模型与创作类型不匹配")
	}
	if selected.NativeRequest != nil {
		var legacy map[string]any
		if a.ImageParameters != "" && (json.Unmarshal([]byte(a.ImageParameters), &legacy) != nil || len(legacy) > 0) {
			return BadAuthRequest("请使用所选模型的原生参数表单")
		}
		var parameters map[string]any
		if a.NativeParameters != "" && (json.Unmarshal([]byte(a.NativeParameters), &parameters) != nil || parameters == nil) {
			return BadAuthRequest("原生参数格式无效")
		}
		parameters, err := studioAgentNativeParameters(*a, selected, parameters)
		if err != nil {
			return err
		}
		input := canvasGenerationInput{Config: providerConfig{NativeParameters: parameters, NativeVariantID: a.NativeVariantID}}
		values, err := studioNativeSpecifications(input, selected)
		if err != nil {
			return err
		}
		raw, _ := json.Marshal(values)
		a.NativeParameters, a.NativeVariantID = string(raw), selected.ID
		a.ImageParameters = ""
		// This is only the Agent's local duration budget; the native request
		// keeps the original /duration value and Platform owns all billing.
		if a.Mode == "video" {
			a.Duration = 0
			value, ok := values["/duration"]
			if !ok {
				value, ok = values["/seconds"]
			}
			if ok {
				duration, err := strconv.Atoi(fmt.Sprint(value))
				if err != nil {
					return BadAuthRequest("视频 duration 无法用于本轮时长预算")
				}
				a.Duration = max(0, duration)
			}
			if a.Duration == 0 && state.Request.Budget.MaxVideoSeconds > 0 {
				return BadAuthRequest("本轮设置了视频时长上限，请选择明确的正数 duration 后重新申请")
			}
		}
		return nil
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

func studioAgentNativeParameters(args cloudAgentMediaArgs, selected platformintegration.ModelSelection, supplied map[string]any) (map[string]any, error) {
	if supplied == nil {
		supplied = map[string]any{}
	}
	// Some native fields use a union type. Read only the simple type of each
	// public alias instead of rejecting unrelated structured native parameters.
	var raw struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if json.Unmarshal(selected.Schema, &raw) != nil {
		return nil, BadAuthRequest("模型参数合同无效")
	}
	qualityKeys := []string{"/quality"}
	if args.Mode == "video" {
		qualityKeys = []string{"/resolution", "/metadata/resolution"}
	}
	type parameterAlias struct {
		keys    []string
		value   any
		present bool
	}
	aliases := []parameterAlias{
		{[]string{"/duration", "/seconds"}, args.Duration, args.Duration != 0},
		{[]string{"/aspect_ratio", "/metadata/aspect_ratio", "/ratio", "/generationConfig/imageConfig/aspectRatio", "/size"}, args.Size, args.Size != ""},
		{qualityKeys, args.Quality, args.Quality != ""},
		{[]string{"/resolution", "/metadata/resolution", "/image_size", "/generationConfig/imageConfig/imageSize"}, args.ImageResolution, args.ImageResolution != ""},
	}
	if args.VideoGenerateAudio != nil {
		aliases = append(aliases, parameterAlias{[]string{"/generate_audio", "/metadata/generate_audio", "/audio"}, *args.VideoGenerateAudio, true})
	}
	if args.VideoWatermark != nil {
		aliases = append(aliases, parameterAlias{[]string{"/watermark", "/metadata/watermark"}, *args.VideoWatermark, true})
	}
	for _, alias := range aliases {
		if !alias.present {
			continue
		}
		key := ""
		for _, candidate := range alias.keys {
			if _, ok := raw.Properties[candidate]; ok {
				key = candidate
				break
			}
		}
		if key == "" {
			return nil, BadAuthRequest("所选模型未发布该参数，请按模型详情填写 nativeParameters")
		}
		value := alias.value
		var field struct {
			Type string `json:"type"`
		}
		_ = json.Unmarshal(raw.Properties[key], &field)
		if field.Type == "string" {
			value = fmt.Sprint(value)
		}
		if existing, ok := supplied[key]; ok && fmt.Sprint(existing) != fmt.Sprint(value) {
			return nil, BadAuthRequest("原生参数与同名生成参数冲突，请统一后重新确认")
		}
		supplied[key] = value
	}
	return supplied, nil
}

func (s *Service) requestStudioAgentMediaApproval(run *model.CloudAgentExecution, state *cloudAgentRuntime, call cloudAgentCall) error {
	req, plan, err := s.prepareCloudAgentMedia(run, state, cloudAgentMediaCall(call))
	if err != nil {
		return s.cloudAgentMediaError(run, state, "admission", false, false, err)
	}
	selected, err := s.studioAgentMediaSelection(run.UserID, run.ID, req.Model, plan.Args.NativeVariantID)
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
		if selected.GenerationV1 {
			if err := repo.OpenStudioMediaAction(model.StudioMediaAction{RunID: run.ID, ApprovalID: state.Approval.ID, UserID: run.UserID, ProjectID: state.Request.CanvasID, NodeID: plan.Args.NodeID}); err != nil {
				return err
			}
		}

		state.PiResumePrompt = ""
		current.Status = "waiting_approval"
		state.event(run.ID, "approval_requested", map[string]any{"approvalId": state.Approval.ID, "toolName": call.Function.Name, "modelName": selected.Name, "arguments": json.RawMessage(call.Function.Arguments), "preview": preview, "studioMedia": true, "text": preview.Description})
		return cloudAgentSave(current, state)
	})
}

// An automatic turn uses the model contract frozen under its execution grant.
// The original call hash lets Pi reconnect to the same task after a lost reply.
func (s *Service) submitAutomaticStudioMedia(run *model.CloudAgentExecution, state *cloudAgentRuntime, call, original cloudAgentCall) error {
	req, plan, err := s.prepareCloudAgentMedia(run, state, cloudAgentMediaCall(call))
	if err != nil {
		return s.cloudAgentMediaError(run, state, "admission", false, false, err)
	}
	detail, err := s.studioAgentCatalogDetail(run.UserID, run.ID, req.Model, plan.Args.NativeVariantID)
	if err != nil {
		return err
	}
	selected, err := platformintegration.SelectModel(detail.Model, plan.Args.NativeVariantID)
	if err != nil {
		return err
	}
	body, err := s.studioMediaBody(context.Background(), run.UserID, req, selected)
	if err != nil {
		return s.cloudAgentMediaError(run, state, "admission", false, false, err)
	}
	actionID := fmt.Sprintf("%s-%d-%d", run.ID, state.Step, state.CallIndex)
	task, err := studioAgentMediaTask(run.UserID, run.ID, state.Request.CanvasID, actionID, req, selected)
	if err != nil {
		return err
	}
	e := studioExecutionEnvelope{Task: task, Selection: selected, RequestID: "studio-" + task.ID, AgentRunID: run.ID, ContentType: "application/json", Body: body, ActualUsageBilling: true,
		Consumption: &platformclient.MediaConsumptionConstraints{ExpectedCapabilityRevision: detail.CapabilityRevision, ExpectedConnectionRevision: detail.ConnectionRevision, ExpectedPriceVersion: detail.PriceVersion}}
	if !e.validMediaConsumption() {
		return creationConflict("本轮模型的计费版本不完整，请重新读取模型")
	}
	policy, err := s.RuntimePolicy()
	if err != nil {
		return err
	}
	raw, err := json.Marshal(plan.Args)
	if err != nil {
		return err
	}
	call.Function.Arguments = string(raw)
	state.Calls[state.CallIndex] = call
	state.StudioMediaCallHash = cloudAgentApprovalCallHash(original)
	state.PiResumePrompt = ""
	if selected.GenerationV1 {
		a := model.StudioMediaAction{RunID: run.ID, ApprovalID: actionID, UserID: run.UserID, ProjectID: state.Request.CanvasID, NodeID: plan.Args.NodeID}
		_, err = s.decideStudioGenerationAction(context.Background(), run, state, a, &task, studioAgentGenerationCommand(e), state.StudioMediaCallHash, "", "auto", plan, "", policy, &platformclient.Response{Header: make(http.Header)}, false)
		return err
	}
	frozen, err := s.encryptStudioValue(e)
	if err != nil {
		return err
	}
	s.storageMu.Lock()
	err = s.repo.MutateCloudAgent(run.UserID, run.ID, run.Revision, func(current *model.CloudAgentExecution, repo *repository.Repository) error {
		if (current.Status != "running" && current.Status != "queued") || current.CleanupPending {
			return repository.ErrCreationConflict
		}
		root, err := repo.StudioExecution(run.UserID, run.ID)
		if err != nil {
			return err
		}
		if root.State != "agent" || root.Released {
			return creationConflict("本轮授权已结束")
		}
		if err := createCloudAgentMediaNode(repo, run.UserID, state.Request.CanvasID, plan, &task, policy, cloudAgentCanvasEventRecorder(run.ID, state)); err != nil {
			return err
		}
		if err := queueStudioAgentMedia(repo, task, frozen, state.StudioMediaCallHash, root.AuthorizationEncrypted, policy); err != nil {
			return err
		}
		current.Status = "running"
		state.TaskIDs = append(state.TaskIDs, task.ID)
		state.MediaTaskID = task.ID
		state.Generations++
		state.VideoSeconds += plan.Args.Duration
		state.event(run.ID, "generation_task_created", map[string]any{"toolName": call.Function.Name, "taskId": task.ID, "nodeId": plan.Args.NodeID, "canvasId": state.Request.CanvasID})
		return cloudAgentSave(current, state)
	})
	s.storageMu.Unlock()
	if err == nil {
		s.wakeTaskDispatcher()
	}
	return err
}

func studioAgentMediaTask(userID, runID, canvasID, actionID string, req CreateTaskRequest, selected platformintegration.ModelSelection) (model.Task, error) {
	public, err := studioPublicTaskInput(selected.Kind, req)
	if err != nil {
		return model.Task{}, err
	}
	id := cloudAgentID(userID, runID+":media:"+actionID)
	return model.Task{ID: id, UserID: userID, ProjectID: canvasID, AgentRunID: runID, Type: req.Type, Operation: req.Operation, Provider: "platform", Model: selected.Name, Prompt: req.Prompt, InputJSON: public, Status: model.TaskStatusQueued, Stage: "等待平台媒体任务"}, nil
}

func queueStudioAgentMedia(repo *repository.Repository, task model.Task, frozen, intent, grant string, policy RuntimePolicySetting) error {
	execution, err := repo.CreateStudioExecution(model.StudioExecution{TaskID: task.ID, UserID: task.UserID, CommandID: "agent-media:" + task.ID, IntentHash: intent, EnvelopeEncrypted: frozen, State: "authorizing", Released: true})
	if err != nil {
		return err
	}
	if execution.IntentHash != intent {
		return creationConflict("该媒体步骤已有其他请求")
	}
	return repo.QueueStudioExecution(task, grant, policy.Task.ActiveTaskLimit)
}

func (s *Service) QuoteStudioAgentMedia(ctx context.Context, userID string, session platformclient.SessionContext, runID, approvalID string, prepareActualUsage bool) (*StudioAgentMediaQuote, *platformclient.Response, error) {
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
	if prepareActualUsage && state.Approval.StudioQuote != nil && state.Approval.StudioQuote.BillingMode != "actual_usage" {
		return nil, response, creationConflict("历史草稿须使用原消费授权")
	}
	req, plan, err := s.prepareCloudAgentMedia(run, &state, state.Approval.Call)
	if err != nil {
		return nil, response, err
	}
	root, err := s.repo.StudioExecution(userID, runID)
	if err != nil {
		return nil, response, err
	}
	var frozen studioExecutionEnvelope
	if err := s.decryptStudioValue(root.EnvelopeEncrypted, &frozen); err != nil {
		return nil, response, err
	}
	for _, summary := range frozen.CatalogSummaries {
		if summary.ModelName == req.Model {
			req.Platform = &StudioTaskOptions{VariantID: plan.Args.NativeVariantID, ExpectedPriceVersion: summary.PriceVersion, ExpectedCapabilityRevision: summary.CapabilityRevision, ExpectedConnectionRevision: summary.ConnectionRevision}
			break
		}
	}
	selected, err := s.studioSelectTaskModel(ctx, userID, session, req, response)
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
	task, err := studioAgentMediaTask(userID, runID, state.Request.CanvasID, approvalID, req, selected)
	if err != nil {
		return nil, response, err
	}
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
	if !prepareActualUsage {
		// Reading an estimate must not create approval authority. A cancelled
		// confirmation or reconnect leaves the original approval unchanged.
		return &StudioAgentMediaQuote{EstimatedPoints: preview.WalletPoints, PriceVersion: preview.PriceVersion, Parameters: parameters}, response, nil
	}
	quote := StudioAgentMediaQuote{ID: newID(), BillingMode: "actual_usage", EstimatedPoints: preview.WalletPoints, PriceVersion: preview.PriceVersion, ExpiresAt: time.Now().Add(cloudAgentMediaQuoteLifetime), Parameters: parameters}
	prepared := studioAgentMediaPrepared{Envelope: studioExecutionEnvelope{Task: task, Selection: selected, RequestID: "studio-" + task.ID, AgentRunID: runID, ContentType: "application/json", Body: body, Consumption: studioMediaConsumption(preview, req.Platform), ActualUsageBilling: true}, Request: req, Plan: *plan, DependencyHash: dependency, Quote: quote}
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
func (s *Service) DecideStudioAgentMedia(userID, runID, approvalID, decision, reason, quoteID string, settings *CloudAgentMediaSettings, enableAuto ...bool) error {
	promote := len(enableAuto) == 1 && enableAuto[0]
	if promote && decision != "approve" {
		return BadAuthRequest("切换模式需要指定本轮待执行的操作")
	}
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
		if promote && !state.StudioAutoMedia {
			return creationConflict("该操作已在其他模式下处理")
		}
		if previous == "manual" {
			return nil
		}

		if state.DecisionPreparedHashes[approvalID] == "" && quoteID == "" {
			return s.DecideCloudAgentApproval(userID, runID, approvalID, decision, reason, settings)
		}
		if previous == decision && quoteID != "" && state.DecisionPreparedHashes[approvalID] == quoteID {
			return nil
		}
		return cloudAgentDecisionConflict(previous)
	}
	if state.Approval == nil || !state.Approval.StudioMedia {
		return s.decideCloudAgentApproval(userID, runID, approvalID, decision, reason, promote, settings)
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
	if !prepared.Envelope.validMediaConsumption() || prepared.Envelope.ActualUsageBilling != (prepared.Quote.BillingMode == "actual_usage") {
		return creationConflict("原媒体消费版本未冻结，请重新确认报价")
	}
	body, err := s.studioMediaBody(context.Background(), userID, prepared.Request, prepared.Envelope.Selection)
	if err != nil {
		return err
	}
	if !bytes.Equal(body, prepared.Envelope.Body) {
		return creationConflict("参考素材已变化，请重新申请报价")
	}
	if promote {
		if state.Request.PermissionMode == "read_only" || !cloudAgentToolAllowed(state.Request, state.Approval.Call.Function.Name) {
			return creationConflict("本轮未获生成权限")
		}
		state.Request.PermissionMode = "auto"
		state.StudioAutoMedia = true
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
	if prepared.Envelope.Selection.GenerationV1 {
		return s.approveStudioGeneration(run, &state, &prepared, quoteID, reason, policy, followUp)
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
		if err := queueStudioAgentMedia(repo, task, frozen, quoteID, root.AuthorizationEncrypted, policy); err != nil {
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
