package repository

import (
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"yingce/backend/internal/model"
)

// SyncPlatformUser stores a product identity reference, never an authentication
// session, password, wallet or cached administrator grant.
func (r *Repository) SyncPlatformUser(user model.User, identity model.UserIdentity) (*model.User, error) {
	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoNothing: true}).Create(&user).Error; err != nil {
			return err
		}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "provider"}, {Name: "subject"}}, DoNothing: true}).Create(&identity).Error; err != nil {
			return err
		}
		var stored model.UserIdentity
		if err := tx.Where("provider = ? AND subject = ?", identity.Provider, identity.Subject).First(&stored).Error; err != nil {
			return err
		}
		if stored.UserID != user.ID {
			return errors.New("Platform identity binding conflict")
		}
		return tx.Model(&model.User{}).Where("id = ?", user.ID).Updates(map[string]any{
			"display_name": user.DisplayName, "email": user.Email, "role": model.UserRoleUser,
			"status": model.UserStatusActive, "password_hash": "", "updated_at": time.Now(),
		}).Error
	})
	if err != nil {
		return nil, err
	}
	return r.User(user.ID)
}
