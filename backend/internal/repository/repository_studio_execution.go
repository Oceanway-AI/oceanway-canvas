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
