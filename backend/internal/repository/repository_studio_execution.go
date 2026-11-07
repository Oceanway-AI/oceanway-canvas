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
		result := tx.Model(&model.StudioExecution{}).Where("task_id = ? AND user_id = ? AND state = ?", task.ID, task.UserID, "authorizing").Updates(map[string]any{"authorization_encrypted": encryptedAuth, "state": "prepared"})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		if err := enforceActiveTaskLimit(tx, task.UserID, activeLimit); err != nil {
			return err
		}
		return tx.Create(&task).Error
	})
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
