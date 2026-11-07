package repository

import (
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"yingce/backend/internal/model"
)

func (r *Repository) StudioMaterialAuthority(userID, sessionHash string) (*model.StudioMaterialAuthority, error) {
	var row model.StudioMaterialAuthority
	err := r.db.First(&row, "user_id = ? AND session_hash = ? AND expires_at > ?", userID, sessionHash, time.Now().Add(time.Minute)).Error
	return &row, err
}

func (r *Repository) SaveStudioMaterialAuthority(row model.StudioMaterialAuthority) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("expires_at < ?", time.Now()).Delete(&model.StudioMaterialAuthority{}).Error; err != nil {
			return err
		}
		return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "session_hash"}}, DoUpdates: clause.AssignmentColumns([]string{"authorization_encrypted", "expires_at", "updated_at"})}).Create(&row).Error
	})
}

func (r *Repository) CreateStudioMaterialUpload(in model.StudioMaterialUpload) (*model.StudioMaterialUpload, error) {
	if err := r.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&in).Error; err != nil {
		return nil, err
	}
	return r.StudioMaterialUpload(in.UserID, in.ID)
}

func (r *Repository) StudioMaterialUpload(userID, id string) (*model.StudioMaterialUpload, error) {
	var v model.StudioMaterialUpload
	err := r.db.First(&v, "id = ? AND user_id = ?", id, userID).Error
	return &v, err
}

func (r *Repository) SetStudioMaterialUploadID(userID, id, uploadID string) error {
	result := r.db.Model(&model.StudioMaterialUpload{}).Where("id = ? AND user_id = ? AND (upload_id = '' OR upload_id = ?)", id, userID, uploadID).Update("upload_id", uploadID)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *Repository) StudioMaterialReference(userID, resourceID string) (*model.StudioMaterialReference, error) {
	var v model.StudioMaterialReference
	err := r.db.First(&v, "resource_id = ? AND user_id = ?", resourceID, userID).Error
	return &v, err
}

func (r *Repository) CreateStudioMaterialResource(ref model.StudioMaterialReference, resource model.Resource) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&ref).Error; err != nil {
			return err
		}
		var saved model.StudioMaterialReference
		if err := tx.First(&saved, "resource_id = ? AND user_id = ?", ref.ResourceID, ref.UserID).Error; err != nil {
			return err
		}
		if saved.AssetID != ref.AssetID || saved.VersionID != ref.VersionID || saved.FileID != ref.FileID || saved.SHA256 != ref.SHA256 || saved.ByteLength != ref.ByteLength || saved.OwnerID != ref.OwnerID {
			return errors.New("material version identity changed")
		}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&resource).Error
	})
}

func (r *Repository) SetStudioMaterialBinding(userID, resourceID, bindingID string) error {
	result := r.db.Model(&model.StudioMaterialReference{}).Where("resource_id = ? AND user_id = ? AND (binding_id = '' OR binding_id = ?)", resourceID, userID, bindingID).Update("binding_id", bindingID)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *Repository) ReadyStudioMaterialResource(userID, resourceID string) error {
	return r.db.Model(&model.Resource{}).Where("id = ? AND user_id = ? AND provider = ?", resourceID, userID, "platform").Update("status", model.ResourceStatusReady).Error
}
