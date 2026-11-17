package app

import "yingce/backend/internal/model"

// Root carriers keep text_replay in storage so the generation worker cannot
// claim them. Public task reads use the orchestration's control state.
func (s *Service) projectStudioAgentTasks(userID string, tasks []model.Task) error {
	ids := make([]string, 0)
	for _, task := range tasks {
		if task.Operation == cloudAgentOperation {
			ids = append(ids, task.ID)
		}
	}
	runs, err := s.repo.CloudAgentTaskStates(userID, ids)
	if err != nil {
		return err
	}
	byID := make(map[string]model.CloudAgentExecution, len(runs))
	for _, run := range runs {
		byID[run.ID] = run
	}
	for i := range tasks {
		task := &tasks[i]
		run, ok := byID[task.ID]
		if !ok {
			continue
		}
		task.AgentRunID, task.AgentStatus, task.AgentCleanupPending = run.ID, run.Status, run.CleanupPending
		task.UpdatedAt = run.UpdatedAt
		switch run.Status {
		case "queued":
			task.Status, task.Stage = model.TaskStatusQueued, "等待 Agent 执行"
		case "running":
			task.Status, task.Stage = model.TaskStatusRunning, "Agent 执行中"
		case "waiting_approval":
			task.Status, task.Stage = model.TaskStatusRunning, "等待审批"
		case "completed":
			task.Status, task.Stage, task.Progress = model.TaskStatusSucceeded, "Agent 已完成", 100
		case "failed":
			task.Status, task.Stage, task.Error = model.TaskStatusFailed, "Agent 执行失败", run.FailureMessage
		case "cancelled":
			task.Status, task.Stage = model.TaskStatusCancelled, "Agent 已停止"
		}
		if cloudAgentRunTerminal(run.Status) {
			task.CompletedAt = &run.UpdatedAt
		}
		if run.CleanupPending {
			task.Stage += "，正在核对已提交步骤"
		}
	}
	return nil
}
