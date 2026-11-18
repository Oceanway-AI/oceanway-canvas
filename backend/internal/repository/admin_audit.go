package repository

import "yingce/backend/internal/model"

func (r *Repository) AppendAdminAudit(event *model.AdminAuditEvent) error {
	return r.db.Create(event).Error
}

func (r *Repository) AdminAuditEvents(targetType string, targetID string, limit int, offset int) ([]model.AdminAuditEvent, int64, error) {
	query := r.db.Model(&model.AdminAuditEvent{})
	if targetType != "" {
		query = query.Where("target_type = ?", targetType)
	}
	if targetID != "" {
		query = query.Where("target_id = ?", targetID)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var events []model.AdminAuditEvent
	err := query.Order("created_at desc").Limit(limit).Offset(offset).Find(&events).Error
	return events, total, err
}

func (r *Repository) APICallLog(id string) (*model.ApiCallLog, error) {
	var log model.ApiCallLog
	if err := r.db.First(&log, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &log, nil
}
