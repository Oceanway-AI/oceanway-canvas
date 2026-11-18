package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"yingce/backend/internal/model"
	"yingce/backend/internal/repository"
)

// Task completion owns diagnostics, replay and output registration. Platform
// execution receipts own funds independently of local persistence outcomes.
type taskTerminalCoordinator struct {
	repo              taskTerminalRepository
	replay            taskReplayLifecycle
	logger            taskLifecycleLogger
	outputs           taskOutputLifecycle
	userFacingMessage func(error) string
	logFailedAttempt  func(model.Task, error)
}

type taskTerminalRepository interface {
	Task(id string) (*model.Task, error)
	UpdateTaskTerminalDiagnostic(task *model.Task, completedAt time.Time) (bool, error)
}

type taskReplayLifecycle interface {
	finalizeTaskTextReplay(taskID string, status model.TaskStatus) error
}

type taskLifecycleLogger interface {
	log(userID string, taskID string, level string, message string, payload string) error
}

type taskOutputLifecycle interface {
	RegisterTaskOutputFromTask(task model.Task) error
}

func newTaskTerminalCoordinator(s *Service) *taskTerminalCoordinator {
	return &taskTerminalCoordinator{repo: s.repo, replay: s, logger: s, outputs: s,
		userFacingMessage: s.UserFacingErrorMessage, logFailedAttempt: s.ensureFailedProviderAttemptLogged}
}

func (s *Service) terminalCoordinator() *taskTerminalCoordinator {
	if s.taskTerminalCoordinator != nil {
		return s.taskTerminalCoordinator
	}
	return newTaskTerminalCoordinator(s)
}

func (c *taskTerminalCoordinator) markPreparationFailure(task *model.Task, stage string, err error, submissionUncertain bool) error {
	c.ensureFailedAttemptLogged(task, err)
	task.Status, task.Stage, task.Error = model.TaskStatusFailed, stage, c.userFacingMessage(err)
	outcome, retryClass := "not_submitted", "correct_input"
	if submissionUncertain {
		outcome, retryClass = "unknown", "reconcile_submission"
	}
	recordTaskDiagnostic(task, "preparation", "preparation_failed", outcome, retryClass, err)
	if terminalErr := c.markTerminalState(task); terminalErr != nil {
		return errors.Join(err, terminalErr)
	}
	return err
}

// Cancellation already recorded by the caller is idempotent. A failed
// execution still returns its original error for worker recovery and logging.
func (c *taskTerminalCoordinator) handleExecutionFailure(task *model.Task, err error, providerSucceeded bool, channelSlotFailedBeforeRequest bool) error {
	if errors.Is(err, context.Canceled) {
		if latest, latestErr := c.repo.Task(task.ID); latestErr == nil && latest.Status == model.TaskStatusCancelled {
			return c.handleAlreadyCancelled(*latest)
		}
		task.Status, task.Stage, task.Error = model.TaskStatusCancelled, "任务已取消", "任务已取消"
		now := time.Now()
		task.CancellationSource, task.CancellationRequestedAt = model.TaskCancellationWorkerContext, &now
		recordTaskDiagnostic(task, "cancellation", "task_cancelled", taskSubmissionOutcome(task, channelSlotFailedBeforeRequest), "reconcile_cancellation", err)
		if terminalErr := c.markTerminalState(task); terminalErr != nil {
			return terminalErr
		}
		c.finalizeReplay(task, model.TaskStatusCancelled, "文本回放草稿归并失败")
		_ = c.logger.log(task.UserID, task.ID, "warn", "任务已取消", "")
		return nil
	}

	task.Status, task.Stage, task.Error = model.TaskStatusFailed, "任务失败", c.userFacingMessage(err)
	c.ensureFailedAttemptLogged(task, err)
	outcome := taskSubmissionOutcome(task, channelSlotFailedBeforeRequest)
	if providerSucceeded {
		outcome = "accepted"
	}
	retryClass := "new_generation_authorization"
	if outcome == "unknown" {
		retryClass = "reconcile_submission"
	}
	recordTaskDiagnostic(task, "execution", "provider_execution_failed", outcome, retryClass, err)
	if terminalErr := c.markTerminalState(task); terminalErr != nil {
		return errors.Join(err, terminalErr)
	}
	c.finalizeReplay(task, model.TaskStatusFailed, "文本回放草稿归并失败")
	_ = c.logger.log(task.UserID, task.ID, "error", "任务处理失败", task.Error)
	return err
}

func (c *taskTerminalCoordinator) handleAlreadyCancelled(task model.Task) error {
	c.finalizeReplay(&task, model.TaskStatusCancelled, "文本回放草稿归并失败")
	_ = c.logger.log(task.UserID, task.ID, "warn", "任务已取消，worker 已停止执行", "")
	return nil
}

func (c *taskTerminalCoordinator) handleCancelledResult(task model.Task) error {
	c.finalizeReplay(&task, model.TaskStatusCancelled, "文本回放草稿归并失败")
	_ = c.logger.log(task.UserID, task.ID, "warn", "任务已取消，丢弃生成结果", "")
	return nil
}

// A concurrent cancellation wins over a late result. Otherwise the original
// persistence error remains observable with a recover_result diagnostic.
func (c *taskTerminalCoordinator) handleResultPersistenceFailure(task *model.Task, saveErr error) (handled bool, err error) {
	if errors.Is(saveErr, repository.ErrTaskStateConflict) {
		latest, latestErr := c.repo.Task(task.ID)
		if latestErr == nil && latest.Status == model.TaskStatusCancelled {
			return true, c.handleCancelledResult(*latest)
		}
	}
	task.Status, task.Stage, task.Error = model.TaskStatusFailed, "任务结果保存失败", c.userFacingMessage(saveErr)
	recordTaskDiagnostic(task, "result_persistence", "result_persistence_failed", "accepted", "recover_result", saveErr)
	if terminalErr := c.markTerminalState(task); terminalErr != nil {
		return false, errors.Join(saveErr, terminalErr)
	}
	c.finalizeReplay(task, model.TaskStatusFailed, "文本回放草稿归并失败")
	_ = c.logger.log(task.UserID, task.ID, "error", "任务结果保存失败", task.Error)
	return false, saveErr
}

func (c *taskTerminalCoordinator) ensureFailedAttemptLogged(task *model.Task, err error) {
	if c.logFailedAttempt != nil && task != nil && err != nil {
		c.logFailedAttempt(*task, err)
	}
}

func (c *taskTerminalCoordinator) handleSuccess(task *model.Task) error {
	c.finalizeReplay(task, model.TaskStatusSucceeded, "文本回放窗口更新失败")
	completedTask, err := c.repo.Task(task.ID)
	if err != nil {
		_ = c.logger.log(task.UserID, task.ID, "error", "任务成功但读取任务产物失败", err.Error())
		return fmt.Errorf("任务成功后读取任务产物失败：%w", err)
	}
	if completedTask.MediaRecoveryJSON == "" {
		if err := c.outputs.RegisterTaskOutputFromTask(*completedTask); err != nil {
			_ = c.logger.log(task.UserID, task.ID, "error", "任务成功但项目产物登记失败", err.Error())
			return fmt.Errorf("任务成功后的项目产物登记失败：%w", err)
		}
	}
	_ = c.logger.log(task.UserID, task.ID, "info", "任务完成，结果已持久化", "")
	return nil
}

func (c *taskTerminalCoordinator) markTerminalState(task *model.Task) error {
	completedAt := time.Now()
	task.CompletedAt = &completedAt
	updated, err := c.repo.UpdateTaskTerminalDiagnostic(task, completedAt)
	if err != nil {
		return fmt.Errorf("写入任务终态失败：%w", err)
	}
	if !updated {
		return repository.ErrTaskStateConflict
	}
	if leases, ok := c.repo.(interface{ ReleaseCloudAgentResourceLeases(string, string) error }); ok && task.ID != "" {
		if err := leases.ReleaseCloudAgentResourceLeases(task.UserID, "task:"+task.ID); err != nil {
			return fmt.Errorf("释放任务资源租约失败：%w", err)
		}
	}
	return nil
}

func (c *taskTerminalCoordinator) finalizeReplay(task *model.Task, status model.TaskStatus, message string) {
	if err := c.replay.finalizeTaskTextReplay(task.ID, status); err != nil {
		_ = c.logger.log(task.UserID, task.ID, "error", message, err.Error())
	}
}
