package repository

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"yingce/backend/internal/model"
)

func (r *Repository) StudioExecution(userID, taskID string) (*model.StudioExecution, error) {
	var v model.StudioExecution
	err := r.db.First(&v, "task_id = ? AND user_id = ?", taskID, userID).Error
	return &v, err
}

func (r *Repository) StudioExecutionResponses(userID string, taskIDs []string) ([]model.StudioExecution, error) {
	var rows []model.StudioExecution
	err := r.db.Select("task_id", "response_encrypted").Where("user_id = ? AND task_id IN ?", userID, taskIDs).Find(&rows).Error
	return rows, err
}

func (r *Repository) CreateStudioExecution(v model.StudioExecution) (*model.StudioExecution, error) {
	if err := r.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&v).Error; err != nil {
		return nil, err
	}
	return r.StudioExecution(v.UserID, v.TaskID)
}

// Queue and authority publish together. Concurrent same-command admissions may
// recover the same EXA but can never replace the original encrypted envelope.
func (r *Repository) QueueStudioExecution(task model.Task, encryptedAuth string, activeLimit int) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		state := "prepared"
		if task.Operation == "cloud_agent" && task.Status == model.TaskStatusTextReplay {
			state = "agent"
		}
		result := tx.Model(&model.StudioExecution{}).Where("task_id = ? AND user_id = ? AND state = ?", task.ID, task.UserID, "authorizing").Updates(map[string]any{"authorization_encrypted": encryptedAuth, "state": state})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		return New(tx).CreateTaskWithActiveLimit(&task, activeLimit)
	})
}

// A run owns the EXA. Children never release it, and a terminal run still waits
// for every submitted child to recover before the shared authority is released.
func (r *Repository) CompleteStudioAgentExecutions() error {
	return r.db.Model(&model.StudioExecution{}).
		Where("state = ?", "agent").
		Where("EXISTS (SELECT 1 FROM cloud_agent_executions a WHERE a.id = studio_executions.task_id AND a.status IN ? AND a.cleanup_pending = ?)", []string{"completed", "failed", "cancelled", "rejected"}, false).
		Where("NOT EXISTS (SELECT 1 FROM tasks t WHERE t.agent_run_id = studio_executions.task_id AND t.status NOT IN ?)", []model.TaskStatus{model.TaskStatusSucceeded, model.TaskStatusFailed, model.TaskStatusCancelled}).
		Update("state", "completed").Error
}

func (r *Repository) StudioAgentRootsWithoutRun() ([]model.Task, error) {
	var tasks []model.Task
	err := r.db.Model(&model.Task{}).Joins("JOIN studio_executions s ON s.task_id = tasks.id").
		Where("s.state = ? AND NOT EXISTS (SELECT 1 FROM cloud_agent_executions a WHERE a.id = tasks.id)", "agent").Limit(50).Find(&tasks).Error
	return tasks, err
}

func (r *Repository) StopPreparedStudioAgentTasks(userID, runID string) error {
	var tasks []model.Task
	if err := r.db.Where("user_id = ? AND agent_run_id = ? AND status IN ?", userID, runID, []model.TaskStatus{model.TaskStatusQueued, model.TaskStatusRunning}).Find(&tasks).Error; err != nil {
		return err
	}
	for _, task := range tasks {
		if err := r.db.Transaction(func(tx *gorm.DB) error {
			locked := tx.Model(&model.Task{}).Where("id = ? AND status IN ?", task.ID, []model.TaskStatus{model.TaskStatusQueued, model.TaskStatusRunning}).Update("updated_at", time.Now())
			if locked.Error != nil || locked.RowsAffected == 0 {
				return locked.Error
			}
			result := tx.Model(&model.StudioExecution{}).Where("task_id = ? AND state = ?", task.ID, "prepared").Update("state", "completed")
			if result.Error != nil || result.RowsAffected == 0 {
				return result.Error
			}
			return tx.Model(&model.Task{}).Where("id = ?", task.ID).Updates(map[string]any{"status": model.TaskStatusCancelled, "stage": "Agent 已停止，任务未提交", "completed_at": time.Now(), "lease_owner": "", "lease_expires_at": nil}).Error
		}); err != nil {
			return err
		}
	}
	return nil
}

// Lock the current task lease and change execution state in the same transaction.
// Only the winner of prepared -> submitted may perform the paid POST.
func (r *Repository) SubmitStudioExecution(task *model.Task) (bool, error) {
	changed := false
	err := r.db.Transaction(func(tx *gorm.DB) error {
		lock := taskLeaseWriter(tx.Model(&model.Task{}), task.LeaseOwner).Where("id = ? AND status = ?", task.ID, model.TaskStatusRunning).Update("updated_at", time.Now())
		if lock.Error != nil {
			return lock.Error
		}
		if lock.RowsAffected != 1 {
			return ErrTaskStateConflict
		}
		result := tx.Model(&model.StudioExecution{}).Where("task_id = ? AND state = ?", task.ID, "prepared").Update("state", "submitted")
		changed = result.RowsAffected == 1
		return result.Error
	})
	return changed, err
}

// Serialize retry reservation and dispatch with cancellation and replacement
// of the active Pi step. Querying an already submitted request needs no such gate.
func (r *Repository) WithActiveStudioAgentStep(task *model.Task, fn func(*Repository) error) (bool, error) {
	active := false
	err := r.Transaction(func(repo *Repository) error {
		var parent model.CloudAgentExecution
		row := repo.db.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").
			Where("id = ? AND user_id = ? AND active_task_id = ? AND status IN ? AND cleanup_pending = ?", task.AgentRunID, task.UserID, task.ID, []string{"queued", "running"}, false).
			Limit(1).Find(&parent)
		if row.Error != nil || row.RowsAffected == 0 {
			return row.Error
		}
		active = true
		return fn(repo)
	})
	return active, err
}

func (r *Repository) PrepareStudioAgentTextRetry(task *model.Task, previous, next string, delay time.Duration) (bool, error) {
	return r.WithActiveStudioAgentStep(task, func(repo *Repository) error {
		if err := repo.DeferRunningTaskForProviderPoll(task.ID, task.LeaseOwner, "平台确认本次未执行，等待重试", delay); err != nil {
			return err
		}
		updated := repo.db.Model(&model.StudioExecution{}).
			Where("task_id = ? AND user_id = ? AND state = ? AND envelope_encrypted = ? AND response_encrypted = ''", task.ID, task.UserID, "submitted", previous).
			Updates(map[string]any{"state": "prepared", "envelope_encrypted": next})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return ErrTaskStateConflict
		}
		return repo.UpdateTaskExecutionDiagnostic(task.UserID, task.ID, "")
	})
}

func (r *Repository) SaveStudioTextRequestID(task *model.Task, requestID string) error {
	write := taskLeaseWriter(r.db.Model(&model.Task{}), task.LeaseOwner).Where("id = ? AND user_id = ? AND status = ?", task.ID, task.UserID, model.TaskStatusRunning).Update("provider_request_id", requestID)
	if write.Error != nil {
		return write.Error
	}
	if write.RowsAffected != 1 {
		return ErrTaskStateConflict
	}
	task.ProviderRequestID = requestID
	return nil
}

func (r *Repository) SaveStudioExecutionResponse(task *model.Task, encrypted string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		lock := taskLeaseWriter(tx.Model(&model.Task{}), task.LeaseOwner).Where("id = ? AND status = ?", task.ID, model.TaskStatusRunning).Update("updated_at", time.Now())
		if lock.Error != nil {
			return lock.Error
		}
		if lock.RowsAffected != 1 {
			return ErrTaskStateConflict
		}
		return tx.Model(&model.StudioExecution{}).Where("task_id = ? AND state = ? AND response_encrypted = ''", task.ID, "submitted").Update("response_encrypted", encrypted).Error
	})
}

// Replace only the receipt read by this lease owner. A stale poll cannot erase
// a newer original-task observation or a completed execution.
func (r *Repository) RefreshStudioExecutionResponse(task *model.Task, previous, encrypted string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		lock := taskLeaseWriter(tx.Model(&model.Task{}), task.LeaseOwner).Where("id = ? AND status = ?", task.ID, model.TaskStatusRunning).Update("updated_at", time.Now())
		if lock.Error != nil {
			return lock.Error
		}
		if lock.RowsAffected != 1 {
			return ErrTaskStateConflict
		}
		result := tx.Model(&model.StudioExecution{}).Where("task_id = ? AND state = ? AND response_encrypted = ?", task.ID, "submitted", previous).Update("response_encrypted", encrypted)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrTaskStateConflict
		}
		return nil
	})
}

func (r *Repository) CompleteStudioExecution(taskID string) error {
	return r.db.Model(&model.StudioExecution{}).Where("task_id = ?", taskID).Update("state", "completed").Error
}

func (r *Repository) StudioExecutionsToRelease() ([]model.StudioExecution, error) {
	var rows []model.StudioExecution
	err := r.db.Where("state = ? AND released = ?", "completed", false).Limit(50).Find(&rows).Error
	return rows, err
}

func (r *Repository) ReleaseStudioExecution(taskID string) error {
	return r.db.Model(&model.StudioExecution{}).Where("task_id = ? AND state = ?", taskID, "completed").Update("released", true).Error
}

// The insert winner owns the single dispatch. Persist the request and its
// product identity together before any network I/O, including on restart.
func (r *Repository) RegisterStudioGeneration(execution model.StudioExecution, task *model.Task) (bool, error) {
	var first bool
	err := r.db.Transaction(func(tx *gorm.DB) error {
		put := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&execution)
		if put.Error != nil {
			return put.Error
		}
		if put.RowsAffected == 0 {
			var previous model.StudioExecution
			if err := tx.First(&previous, "task_id = ? AND user_id = ?", task.ID, task.UserID).Error; err != nil {
				return err
			}
			if previous.IntentHash != execution.IntentHash {
				return ErrTaskStateConflict
			}
			return nil
		}
		first = true
		return tx.Create(task).Error
	})
	return first, err
}

func (r *Repository) SaveStudioGenerationProjection(task *model.Task, receipt string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		w := tx.Model(&model.Task{}).Where("id = ? AND user_id = ? AND platform_generation_request = ? AND (provider_request_id = '' OR provider_request_id = ?) AND (platform_revision < ? OR (platform_revision = ? AND platform_projection_pending = ?))", task.ID, task.UserID, task.PlatformGenerationRequest, task.ProviderRequestID, task.PlatformRevision, task.PlatformRevision, true).
			Updates(map[string]any{"platform_revision": task.PlatformRevision, "provider_request_id": task.ProviderRequestID, "platform_retry_after_ms": task.PlatformRetryAfterMS, "result_json": task.ResultJSON, "platform_projection_pending": task.PlatformProjectionPending, "status": task.Status, "progress": task.Progress, "stage": task.Stage, "error": task.Error, "completed_at": task.CompletedAt, "execution_diagnostic_json": task.ExecutionDiagnosticJSON})
		if w.Error != nil || w.RowsAffected == 0 {
			return w.Error
		}
		return tx.Model(&model.StudioExecution{}).Where("task_id = ? AND user_id = ?", task.ID, task.UserID).Update("response_encrypted", receipt).Error
	})
}

func (r *Repository) RejectStudioGeneration(task *model.Task, message, diagnostic string) error {
	return r.db.Model(&model.Task{}).Where("id=? AND user_id=? AND platform_revision=0 AND platform_generation_request=?", task.ID, task.UserID, task.PlatformGenerationRequest).Updates(map[string]any{"status": model.TaskStatusFailed, "stage": "平台未受理", "error": message, "execution_diagnostic_json": diagnostic, "completed_at": time.Now()}).Error
}

func (r *Repository) MarkStudioGenerationUnknown(task *model.Task, diagnostic string) error {
	return r.db.Model(&model.Task{}).Where("id=? AND user_id=? AND platform_revision=0 AND status=? AND platform_generation_request=?", task.ID, task.UserID, model.TaskStatusRunning, task.PlatformGenerationRequest).Updates(map[string]any{"stage": "正在查询原生成请求", "execution_diagnostic_json": diagnostic}).Error
}
