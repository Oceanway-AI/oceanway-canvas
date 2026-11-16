package app

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"gorm.io/gorm"
	"yingce/backend/internal/model"
	"yingce/backend/internal/repository"
)

func (s *Service) approveStudioGeneration(run *model.CloudAgentExecution, state *cloudAgentRuntime, prepared *studioAgentMediaPrepared, quoteID, reason string, policy RuntimePolicySetting, followUp bool) error {
	e := prepared.Envelope
	c := e.Consumption
	command := platformclient.MediaCommand{RequestID: e.RequestID, ProductTaskID: run.ID, Model: e.Selection.Name, VariantID: e.Selection.ID, NativeRequest: e.Body, Consumption: &platformclient.MediaGenerationConstraints{ExpectedCapabilityRevision: c.ExpectedCapabilityRevision, ExpectedConnectionRevision: c.ExpectedConnectionRevision, ExpectedPriceVersion: c.ExpectedPriceVersion, BillingPreference: c.BillingPreference, WalletMaxPoints: c.WalletMaxPoints}}
	a, err := s.repo.StudioMediaAction(run.UserID, run.ID, state.Approval.ID)
	if err != nil {
		return err
	}
	response := &platformclient.Response{Header: make(http.Header)}
	_, err = s.decideStudioGenerationAction(context.Background(), run, state, a, &e.Task, command, quoteID, reason, "approve", &prepared.Plan, prepared.DependencyHash, policy, response, followUp)
	return err
}

func (s *Service) joinStudioGenerationApproval(ctx context.Context, req CreateTaskRequest, task *model.Task, command platformclient.MediaCommand, hash string, response *platformclient.Response) (*model.Task, bool, error) {
	meta, _ := req.Input["metadata"].(map[string]any)
	node, source, runID := stringValue(meta["nodeId"]), stringValue(meta["sourceNodeId"]), stringValue(meta["studioPendingRunId"])
	if node == "" {
		return nil, false, nil
	}
	alias, err := s.repo.StudioActionAlias(task.UserID, req.CommandID)
	if err == nil {
		if alias.IntentHash != hash {
			return nil, true, creationConflict("此任务命令已用于不同的创作意图")
		}
		out, e := s.Task(task.UserID, alias.TaskID)
		return out, true, e
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, true, err
	}
	targets := []string{node}
	if runID != "" && source != "" && source != node {
		targets = append(targets, source)
	}
	a, err := s.repo.StudioNodeMediaAction(task.UserID, task.ProjectID, runID, targets)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, true, err
	}
	if a.Winner != "" {
		out, err := s.joinWinningStudioGeneration(a, req.CommandID, hash)
		return out, true, err
	}
	run, err := s.repo.CloudAgent(task.UserID, a.RunID)
	if err != nil {
		return nil, true, err
	}
	state, err := cloudAgentDecode(run)
	if err != nil {
		return nil, true, err
	}
	if run.Status != "waiting_approval" || state.Approval == nil || state.Approval.ID != a.ApprovalID || !state.Approval.StudioMedia {
		fresh, e := s.repo.StudioMediaAction(a.UserID, a.RunID, a.ApprovalID)
		if e != nil {
			return nil, true, e
		}
		if fresh.Winner != "" {
			out, e := s.joinWinningStudioGeneration(fresh, req.CommandID, hash)
			return out, true, e
		}
		return nil, false, nil
	}
	policy, err := s.RuntimePolicy()
	if err != nil {
		return nil, true, err
	}
	command.ProductTaskID = run.ID
	out, err := s.decideStudioGenerationAction(ctx, run, &state, a, task, command, hash, "", "manual", nil, "", policy, response, true)
	return out, true, err
}

func (s *Service) joinWinningStudioGeneration(a model.StudioMediaAction, manualCommand, hash string) (*model.Task, error) {
	if err := s.repo.JoinStudioMediaAction(model.StudioMediaActionAlias{UserID: a.UserID, CommandID: manualCommand, TaskID: a.TaskID, IntentHash: hash}); err != nil {
		if errors.Is(err, repository.ErrTaskStateConflict) {
			return nil, creationConflict("此任务命令已用于不同的创作意图")
		}
		return nil, err
	}
	return s.Task(a.UserID, a.TaskID)
}

// The run checkpoint, losing command alias, canvas binding and original
// authorization change in one transaction. Only its winner can send the POST.
func (s *Service) decideStudioGenerationAction(ctx context.Context, run *model.CloudAgentExecution, state *cloudAgentRuntime, a model.StudioMediaAction, task *model.Task, command platformclient.MediaCommand, hash, reason, winner string, plan *cloudAgentMediaPlan, dependency string, policy RuntimePolicySetting, response *platformclient.Response, followUp bool) (*model.Task, error) {
	stored, err := s.encryptStudioValue(command)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	task.PlatformGenerationRequest = command.RequestID
	task.PlatformWorkspaceID = command.WorkspaceID
	task.Status = model.TaskStatusRunning
	task.StartedAt = &now
	task.Stage = "正在提交生成"
	task.AgentRunID = run.ID
	var auth platformclient.ExecutionContext
	s.storageMu.Lock()
	err = s.repo.MutateCloudAgent(a.UserID, a.RunID, run.Revision, func(current *model.CloudAgentExecution, repo *repository.Repository) error {
		if current.Status != "waiting_approval" || current.CleanupPending {
			return repository.ErrCreationConflict
		}
		if winner == "approve" && (state.Approval.StudioQuote == nil || !time.Now().Before(state.Approval.StudioQuote.ExpiresAt)) {
			return creationConflict("报价已过期，请刷新")
		}
		root, err := repo.StudioExecution(a.UserID, a.RunID)
		if err != nil {
			return err
		}
		if root.State != "agent" || root.Released {
			return creationConflict("原运行授权已结束")
		}
		auth.Authorization, err = s.decryptSettingSecret(root.AuthorizationEncrypted)
		if err != nil {
			return err
		}
		canvas, err := repo.CanvasProjectForUser(a.UserID, a.ProjectID)
		if err != nil {
			return err
		}
		doc, err := creationDocument(canvas.PayloadJSON)
		if err != nil {
			return err
		}
		var metadata map[string]any
		for _, node := range creationMaps(doc["nodes"]) {
			if stringValue(node["id"]) == a.NodeID {
				metadata, _ = node["metadata"].(map[string]any)
				break
			}
		}
		if metadata == nil || stringValue(metadata["agentDraftRunId"]) != run.ID {
			return creationConflict("原节点已删除或不再等待该动作")
		}
		if plan != nil {
			h, err := cloudAgentMediaDependencyHash(doc, plan.Args)
			if err != nil {
				return err
			}
			if h != dependency {
				return creationConflict("画布内容已变化，请重新申请审批")
			}
			plan.Args.SnapshotHash = cloudAgentMediaContentHash(doc)
			plan.Args.DraftRunID = run.ID
			if err = createCloudAgentMediaNode(repo, a.UserID, a.ProjectID, plan, task, policy, cloudAgentCanvasEventRecorder(run.ID, state)); err != nil {
				return err
			}
		} else {
			before := canvas.PayloadJSON
			metadata["taskId"] = task.ID
			metadata["taskStatus"] = "running"
			metadata["status"] = "loading"
			delete(metadata, "agentDraftRunId")
			if err = saveCloudAgentDocument(repo, canvas, doc, policy); err != nil {
				return err
			}
			if err = emitCloudAgentCanvasChange(repo, run.ID, state, cloudAgentMutationInput{UserID: a.UserID, CanvasID: a.ProjectID, BeforeJSON: before, Operation: "generate_media_submit"}); err != nil {
				return err
			}
		}
		if err = repo.WinStudioMediaAction(a, winner, task.ID, command.RequestID); err != nil {
			return err
		}
		ex := model.StudioExecution{TaskID: task.ID, CommandID: command.RequestID, UserID: a.UserID, IntentHash: hash, State: "generation_v1", EnvelopeEncrypted: stored, AuthorizationEncrypted: root.AuthorizationEncrypted, Released: true}
		first, err := repo.RegisterStudioGeneration(ex, task)
		if err != nil {
			return err
		}
		if !first {
			return repository.ErrCreationConflict
		}
		if winner == "manual" {
			if err = repo.JoinStudioMediaAction(model.StudioMediaActionAlias{UserID: a.UserID, CommandID: command.RequestID, TaskID: task.ID, IntentHash: hash}); err != nil {
				return err
			}
		}
		if state.Decisions == nil {
			state.Decisions = map[string]string{}
		}
		if state.DecisionPreparedHashes == nil {
			state.DecisionPreparedHashes = map[string]string{}
		}
		state.Decisions[a.ApprovalID] = winner
		state.DecisionPreparedHashes[a.ApprovalID] = hash
		state.Approval.Decision = winner
		state.Approval.Reason = reason
		state.MediaTaskID = task.ID
		state.TaskIDs = append(state.TaskIDs, task.ID)
		state.Generations++
		if plan != nil {
			state.VideoSeconds += plan.Args.Duration
		}
		current.Status = "running"
		state.event(a.RunID, "approval_decided", map[string]any{"approvalId": a.ApprovalID, "decision": winner, "requestId": command.RequestID})
		state.event(a.RunID, "generation_task_created", map[string]any{"toolName": "generate_media", "taskId": task.ID, "nodeId": a.NodeID, "requestId": command.RequestID, "canvasId": a.ProjectID})
		return cloudAgentSave(current, state)
	})
	s.storageMu.Unlock()
	if errors.Is(err, repository.ErrCreationConflict) {
		fresh, e := s.repo.StudioMediaAction(a.UserID, a.RunID, a.ApprovalID)
		if e == nil && fresh.Winner != "" {
			if winner == "manual" {
				return s.joinWinningStudioGeneration(fresh, command.RequestID, hash)
			}
			return s.Task(a.UserID, fresh.TaskID)
		}
	}
	if err != nil {
		return nil, err
	}
	call, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	err = s.sendStudioGeneration(call, task, platformclient.MediaGenerationAuth{Execution: &auth, WorkspaceID: command.WorkspaceID}, command, response)
	if followUp {
		s.startApprovedCloudAgentMediaWaiter(a.UserID, a.RunID, task.ID)
	}
	if err != nil {
		return nil, err
	}
	return s.Task(a.UserID, task.ID)
}

func (s *Service) waitStudioAgentGeneration(ctx context.Context, user, run, id string) (*model.Task, error) {
	task, err := s.repo.TaskForUser(user, id)
	if err != nil {
		return nil, err
	}
	if task.PlatformGenerationRequest == "" {
		return s.waitCloudAgentTask(ctx, id)
	}
	if task.AgentRunID != run {
		return nil, NotFound("原运行媒体步骤不存在")
	}
	ex, err := s.repo.StudioExecution(user, id)
	if err != nil {
		return nil, err
	}
	grant, err := s.decryptSettingSecret(ex.AuthorizationEncrypted)
	if err != nil {
		return nil, err
	}
	auth := platformclient.MediaGenerationAuth{Execution: &platformclient.ExecutionContext{Authorization: grant}, WorkspaceID: task.PlatformWorkspaceID}
	for !cloudAgentTaskTerminal(task.Status) {
		delay := time.Duration(task.PlatformRetryAfterMS) * time.Millisecond
		if delay <= 0 {
			delay = 2 * time.Second
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
		g, err := s.readStudioGenerationReceipt(ctx, task, auth)
		if err != nil {
			if _, e := s.studioGenerationRejection(task, err); e != nil {
				return nil, e
			}
		} else {
			if err = s.projectStudioGeneration(ctx, task, auth, g, &platformclient.Response{Header: make(http.Header)}); err != nil {
				return nil, err
			}
		}
		task, err = s.repo.TaskForUser(user, id)
		if err != nil {
			return nil, err
		}
	}
	return task, nil
}
