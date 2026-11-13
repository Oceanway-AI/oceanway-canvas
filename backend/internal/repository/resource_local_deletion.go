package repository

import (
	"errors"
	"strings"

	"gorm.io/gorm/clause"
	"yingce/backend/internal/model"
)

var ErrResourceManagedExternally = errors.New("Platform 管理的资源必须通过绑定释放，不能在本地物理删除")

func requireLocalResourceDeletion(resources []model.Resource) error {
	for _, resource := range resources {
		if strings.EqualFold(strings.TrimSpace(resource.Provider), "platform") {
			return ErrResourceManagedExternally
		}
	}
	return nil
}

// Called inside the deletion transaction, before changing assets, history or
// outbox rows. A caller's earlier snapshot is not deletion authority.
func (r *Repository) requireLocalResourceDeletionIDs(ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	var resources []model.Resource
	query := r.db.Select("id", "provider").Where("id IN ?", ids).Order("id")
	if r.Dialect() == "postgres" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := query.Find(&resources).Error; err != nil {
		return err
	}
	return requireLocalResourceDeletion(resources)
}
