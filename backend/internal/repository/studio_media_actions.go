package repository

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"yingce/backend/internal/model"
)

func (r *Repository) OpenStudioMediaAction(a model.StudioMediaAction) error {
	return r.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&a).Error
}
func (r *Repository) StudioMediaAction(user, run, approval string) (model.StudioMediaAction, error) {
	var a model.StudioMediaAction
	err := r.db.First(&a, "user_id=? AND run_id=? AND approval_id=?", user, run, approval).Error
	return a, err
}
func (r *Repository) StudioNodeMediaAction(user, project, run string, nodes []string) (model.StudioMediaAction, error) {
	var a model.StudioMediaAction
	q := r.db.Where("user_id=? AND project_id=? AND node_id IN ?", user, project, nodes)
	if run != "" {
		q = q.Where("run_id=?", run)
	} else {
		q = q.Where("winner=''")
	}
	err := q.Order("created_at DESC, approval_id DESC").First(&a).Error
	return a, err
}
func (r *Repository) WinStudioMediaAction(a model.StudioMediaAction, winner, task, request string) error {
	updated := r.db.Model(&model.StudioMediaAction{}).Where("user_id=? AND run_id=? AND approval_id=? AND winner=''", a.UserID, a.RunID, a.ApprovalID).Updates(map[string]any{"winner": winner, "task_id": task, "request_id": request})
	if updated.Error != nil {
		return updated.Error
	}
	if updated.RowsAffected != 1 {
		return ErrCreationConflict
	}
	return nil
}
func (r *Repository) StudioActionAlias(user, command string) (model.StudioMediaActionAlias, error) {
	var a model.StudioMediaActionAlias
	err := r.db.First(&a, "user_id=? AND command_id=?", user, command).Error
	return a, err
}
func (r *Repository) JoinStudioMediaAction(a model.StudioMediaActionAlias) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		put := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&a)
		if put.Error != nil || put.RowsAffected == 1 {
			return put.Error
		}
		old, err := New(tx).StudioActionAlias(a.UserID, a.CommandID)
		if err != nil {
			return err
		}
		if old.TaskID != a.TaskID || old.IntentHash != a.IntentHash {
			return ErrTaskStateConflict
		}
		return nil
	})
}
