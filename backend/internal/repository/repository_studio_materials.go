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

// Fetch both halves of the immutable identity for the current library page.
type StudioBoundMaterialResource struct {
	Reference model.StudioMaterialReference `gorm:"embedded;embeddedPrefix:bound_"`
	Resource  model.Resource                `gorm:"embedded"`
}

func (r *Repository) StudioBoundMaterialResources(userID string, ids []string) (map[string]StudioBoundMaterialResource, error) {
	out := make(map[string]StudioBoundMaterialResource)
	if len(ids) == 0 {
		return out, nil
	}
	var rows []StudioBoundMaterialResource
	err := r.db.Table("resources AS r").
		Select(`r.id, r.user_id, r.kind, r.mime_type, r.width, r.height, r.duration_ms,
		       ref.asset_id AS bound_asset_id, ref.version_id AS bound_version_id, ref.file_id AS bound_file_id,
		       ref.owner_id AS bound_owner_id, ref.sha256 AS bound_sha256, ref.byte_length AS bound_byte_length`).
		Joins("JOIN studio_material_references AS ref ON ref.resource_id = r.id AND ref.user_id = r.user_id").
		Where("r.user_id = ? AND r.id IN ? AND ref.binding_id <> '' AND r.provider = ? AND r.status = ?", userID, ids, "platform", model.ResourceStatusReady).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.Resource.ID] = row
	}
	return out, nil
}

func (r *Repository) UpdateStudioOriginalMetadata(resource *model.Resource, width, height int, duration int64) error {
	// Independent fields may arrive on different reads. An incomplete or stale
	// observation must never overwrite an already-known pair or duration with zero.
	columns := map[string]any{}
	if width > 0 && height > 0 {
		columns["width"] = width
		columns["height"] = height
	}
	if duration > 0 {
		columns["duration_ms"] = duration
	}
	if len(columns) == 0 {
		return nil
	}
	var written model.Resource
	result := r.db.Model(&written).Clauses(clause.Returning{Columns: []clause.Column{{Name: "width"}, {Name: "height"}, {Name: "duration_ms"}}}).
		Where("user_id = ? AND id = ? AND provider = ?", resource.UserID, resource.ID, "platform").UpdateColumns(columns)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return gorm.ErrRecordNotFound
	}
	resource.Width, resource.Height, resource.DurationMs = written.Width, written.Height, written.DurationMs
	return nil
}
