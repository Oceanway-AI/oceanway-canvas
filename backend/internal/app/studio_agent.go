package app

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"gorm.io/gorm"
	"yingce/backend/internal/model"
	"yingce/backend/internal/platformintegration"
	"yingce/backend/internal/repository"
)

// The session exists only during root admission. Pi receives the original EXA
// through encrypted server records, never a browser cookie or product key.
func (s *Service) CreateStudioCloudAgentRun(ctx context.Context, userID string, session platformclient.SessionContext, req CloudAgentRequest, parentID string) (*CloudAgentRun, *platformclient.Response, error) {
	response := &platformclient.Response{Header: make(http.Header)}
	if req.ChannelID != StudioPlatformChannelID || req.LogicalModelID != "" {
		return nil, response, BadAuthRequest("请选择已发布的平台文本模型")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req.PlatformTextCapability = nil
	// Admission retries recover the original encrypted authority even when the
	// current catalog is unavailable or has changed since the first request.
	existing, lookupErr := s.repo.StudioExecution(userID, cloudAgentID(userID, req.IdempotencyKey))
	if lookupErr == nil {
		var frozen studioExecutionEnvelope
		if err := s.decryptStudioValue(existing.EnvelopeEncrypted, &frozen); err != nil {
			return nil, response, err
		}
		req.PlatformTextCapability = frozen.Selection.TextCapability
		if req.ChannelModelKey == "" {
			req.ChannelModelKey = frozen.Selection.Name
		}
		view, err := s.createCloudAgentRun(userID, req, parentID, func(user string, root CreateTaskRequest) (*model.Task, error) {
			return s.studioCreateAgentRoot(ctx, user, session, root, response, platformclient.Catalog{}, nil, "")
		})
		return view, response, err
	}
	if !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
		return nil, response, lookupErr
	}
	query := platformclient.ModelDetailQuery{ModelName: req.ChannelModelKey, ExpectedPriceVersion: req.ExpectedPriceVersion, ExpectedCapabilityRevision: req.ExpectedCapabilityRevision, ExpectedConnectionRevision: req.ExpectedConnectionRevision}
	if query.ModelName == "" || query.ExpectedPriceVersion == "" {
		summaries, _, err := studioReadCatalogSummaries(platformclient.ModelSummaryQuery{ModelType: "text", Query: query.ModelName, DefaultsOnly: query.ModelName == "", Limit: 50}, func(q platformclient.ModelSummaryQuery) (platformclient.Result[platformclient.ModelSummaryPage], error) {
			result, err := s.studioPlatform.Client.ModelSummaries(ctx, session, q)
			result.Response.AppendCookies(response.Header)
			return result, err
		}, func(m platformclient.ModelSummary) bool {
			return (query.ModelName == "" && m.IsDefault || m.ModelName == query.ModelName) && m.InvocationStatus == "published_text"
		})
		if err != nil {
			return nil, response, studioCatalogError(err)
		}
		if len(summaries) == 0 || summaries[0].PriceVersion == "" {
			return nil, response, BadAuthRequest("未找到当前可用的平台文本模型，请重新选择")
		}
		query.ModelName = summaries[0].ModelName
		if query.ExpectedPriceVersion == "" {
			query.ExpectedPriceVersion = summaries[0].PriceVersion
		}
		if query.ExpectedCapabilityRevision == "" {
			query.ExpectedCapabilityRevision = summaries[0].CapabilityRevision
		}
		if query.ExpectedConnectionRevision == 0 {
			query.ExpectedConnectionRevision = summaries[0].ConnectionRevision
		}
	}
	detail, err := s.studioPlatform.Client.ModelDetail(ctx, session, query)
	detail.Response.AppendCookies(response.Header)
	if err != nil {
		return nil, response, studioCatalogError(err)
	}
	if err := studioValidateCatalogDetail(detail.Data, query); err != nil {
		return nil, response, err
	}
	if detail.Data.Model.ModelType != "text" {
		return nil, response, BadAuthRequest("所选模型不是文本模型")
	}
	req.ChannelModelKey = query.ModelName
	req.PlatformTextCapability = detail.Data.Model.TextCapability()
	catalog := platformclient.Catalog{Items: []platformclient.Model{detail.Data.Model}}
	view, err := s.createCloudAgentRun(userID, req, parentID, func(user string, root CreateTaskRequest) (*model.Task, error) {
		return s.studioCreateAgentRoot(ctx, user, session, root, response, catalog, nil, detail.Data.CatalogRevision)
	})
	return view, response, err
}

func (s *Service) studioCreateAgentRoot(ctx context.Context, userID string, session platformclient.SessionContext, req CreateTaskRequest, response *platformclient.Response, catalog platformclient.Catalog, summaries []platformclient.ModelSummary, revision string) (*model.Task, error) {
	if s.IsDraining() {
		return nil, NewAppError(503, "服务正在维护，请稍后查询原轮次")
	}
	id := req.admission.ID
	initial := req.Input["cloudAgent"].(cloudAgentState)
	hash := initial.Fingerprint
	execution, err := s.repo.StudioExecution(userID, id)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if err == nil {
		if execution.IntentHash != hash {
			return nil, NewAppError(409, "此轮次标识已用于不同请求")
		}
		if execution.State != "authorizing" {
			return s.repo.TaskForUser(userID, id)
		}
	} else {
		var e error
		var selected platformintegration.ModelSelection
		for _, m := range catalog.Items {
			if m.ModelName == initial.Request.ChannelModelKey {
				selected, e = platformintegration.SelectModel(m, "")
				if e != nil {
					return nil, BadAuthRequest(e.Error())
				}
				break
			}
		}
		if selected.Kind != "text" {
			return nil, BadAuthRequest("所选平台文本模型暂不可用")
		}
		// Validate the same canonical boundary that every later step will use.
		if _, err := studioTextBody(req, selected); err != nil {
			return nil, err
		}
		raw, e := json.Marshal(req.Input)
		if e != nil {
			return nil, e
		}
		task := model.Task{ID: id, UserID: userID, ProjectID: req.ProjectID, Type: req.Type, Operation: cloudAgentOperation, Provider: "platform", Model: selected.Name, Prompt: req.Prompt, InputJSON: string(raw), Status: model.TaskStatusTextReplay, Stage: "Agent 控制面载体"}
		envelope, e := s.encryptStudioValue(studioExecutionEnvelope{Task: task, Selection: selected, AgentRunID: id, CatalogVersion: platformclient.ModelCatalogVersion, CatalogRevision: revision, CatalogSummaries: summaries, OnDemandMedia: true})
		if e != nil {
			return nil, e
		}
		execution, e = s.repo.CreateStudioExecution(model.StudioExecution{TaskID: id, UserID: userID, CommandID: "agent:" + id, IntentHash: hash, EnvelopeEncrypted: envelope, State: "authorizing"})
		if e != nil {
			return nil, e
		}
		if execution.IntentHash != hash {
			return nil, NewAppError(409, "此轮次标识已用于不同请求")
		}
	}
	var envelope studioExecutionEnvelope
	if err := s.decryptStudioValue(execution.EnvelopeEncrypted, &envelope); err != nil {
		return nil, err
	}
	grant, err := s.studioPlatform.Client.IssueExecution(ctx, session, id)
	grant.Response.AppendCookies(response.Header)
	if err != nil {
		return nil, WrapAppError(503, "本轮授权尚未确认，请继续查询原轮次", err)
	}
	if grant.Data.AuthorizationID == "" || grant.Data.TaskReference != id || grant.Data.Product != "studio" || grant.Data.RevokedAt != 0 || grant.Data.ExpiresAt <= time.Now().Unix() {
		return nil, NewAppError(409, "本轮原授权已失效，未提交模型请求")
	}
	auth, err := s.encryptSettingSecret(grant.Data.AuthorizationID)
	if err != nil {
		return nil, err
	}
	policy, err := s.RuntimePolicy()
	if err != nil {
		return nil, err
	}
	if err := s.repo.QueueStudioExecution(envelope.Task, auth, policy.Task.ActiveTaskLimit); err != nil {
		return nil, err
	}
	return s.repo.TaskForUser(userID, id)
}

func (s *Service) enqueueStudioAgentTask(run *model.CloudAgentExecution, state *cloudAgentRuntime, req CreateTaskRequest, media *cloudAgentMediaPlan) error {
	if media != nil {
		return creationConflict("媒体步骤必须通过已确认的平台报价提交")
	}
	if req.Type != "canvas_text" || req.Model != state.Request.ChannelModelKey || req.ProjectID != state.Request.CanvasID {
		return BadAuthRequest("模型步骤与本轮授权不一致")
	}
	root, err := s.repo.StudioExecution(run.UserID, run.ID)
	if err != nil {
		return err
	}
	if root.State != "agent" || root.Released {
		return NewAppError(409, "本轮执行授权已结束")
	}
	var authority studioExecutionEnvelope
	if err := s.decryptStudioValue(root.EnvelopeEncrypted, &authority); err != nil {
		return err
	}
	authorization, err := s.decryptSettingSecret(root.AuthorizationEncrypted)
	if err != nil {
		return err
	}
	materials := s.studioPlatform.Client.WorkerMaterials(platformclient.MaterialContext{ExecutionAuthorization: authorization})
	body, err := s.studioTextBodyWithMaterials(context.Background(), run.UserID, materials, req, authority.Selection)
	if err != nil {
		return err
	}
	id := cloudAgentID(run.UserID, fmt.Sprintf("%s:task:%d", run.ID, len(state.TaskIDs)))
	public, _ := json.Marshal(map[string]any{"mode": "text", "metadata": studioTaskMetadata(req.Input)})
	task := model.Task{ID: id, UserID: run.UserID, ProjectID: req.ProjectID, AgentRunID: run.ID, Type: req.Type, Operation: req.Operation, Provider: "platform", Model: authority.Selection.Name, Prompt: req.Prompt, InputJSON: string(public), Status: model.TaskStatusQueued, Stage: "等待平台模型步骤"}
	envelope, err := s.encryptStudioValue(studioExecutionEnvelope{Task: task, Selection: authority.Selection, AgentRunID: run.ID, RequestID: "studio-" + id, ContentType: "application/json", Body: body})
	if err != nil {
		return err
	}
	policy, err := s.RuntimePolicy()
	if err != nil {
		return err
	}
	return s.repo.MutateCloudAgent(run.UserID, run.ID, run.Revision, func(current *model.CloudAgentExecution, repo *repository.Repository) error {
		if cloudAgentRunTerminal(current.Status) || current.CleanupPending {
			return NewAppError(409, "本轮已经停止")
		}
		hash := fmt.Sprintf("%x", sha256.Sum256(body))
		created, err := repo.CreateStudioExecution(model.StudioExecution{TaskID: id, UserID: run.UserID, CommandID: "agent-step:" + id, IntentHash: hash, EnvelopeEncrypted: envelope, State: "authorizing", Released: true})
		if err != nil {
			return err
		}
		if created.IntentHash != hash {
			return NewAppError(409, "模型步骤意图冲突")
		}
		if err := repo.QueueStudioExecution(task, root.AuthorizationEncrypted, policy.Task.ActiveTaskLimit); err != nil {
			return err
		}
		state.TaskIDs = append(state.TaskIDs, task.ID)
		state.ActiveTaskID = task.ID
		if state.ContextCompaction != nil && req.Operation == cloudAgentContextCompactionOperation {
			state.ContextCompaction.Status = "running"
		} else {
			state.Step++
		}
		return cloudAgentSave(current, state)
	})
}
