// 系统渠道、系统设置与方舟私有素材绑定。

package repository

import (
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"yingce/backend/internal/model"
)

func (r *Repository) SystemChannels(includeDisabled bool) ([]model.ModelChannel, error) {
	var channels []model.ModelChannel
	query := r.db.Order("sort_order asc, created_at asc, id asc").Where("scope = ?", model.ChannelScopeSystem)
	if !includeDisabled {
		query = query.Where("enabled = ?", true)
	}
	err := query.Find(&channels).Error
	return channels, err
}

func (r *Repository) HistoricalSystemChannelReferences() ([]model.ModelChannel, error) {
	var channels []model.ModelChannel
	err := r.db.Unscoped().Select("id", "name").Where("scope = ?", model.ChannelScopeSystem).Order("created_at asc").Find(&channels).Error
	return channels, err
}

func (r *Repository) AdminSystemChannelReferences() ([]model.ModelChannel, error) {
	var channels []model.ModelChannel
	err := r.db.Select("id", "name", "enabled").Where("scope = ?", model.ChannelScopeSystem).Order("created_at asc").Find(&channels).Error
	return channels, err
}

func (r *Repository) SystemChannel(id string) (*model.ModelChannel, error) {
	var channel model.ModelChannel
	if err := r.db.First(&channel, "id = ? AND scope = ? AND enabled = ?", id, model.ChannelScopeSystem, true).Error; err != nil {
		return nil, err
	}
	return &channel, nil
}

func (r *Repository) AdminSystemChannel(id string) (*model.ModelChannel, error) {
	var channel model.ModelChannel
	if err := r.db.First(&channel, "id = ? AND scope = ?", id, model.ChannelScopeSystem).Error; err != nil {
		return nil, err
	}
	return &channel, nil
}

func (r *Repository) ApiCallLogs(userID string, admin bool, limit int) ([]model.ApiCallLog, error) {
	var logs []model.ApiCallLog
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	query := r.db.Order("created_at desc").Limit(limit)
	if !admin {
		query = query.Where("user_id = ?", userID)
	}
	err := query.Omit("RequestBody", "ResponseBody").Find(&logs).Error
	return logs, err
}

func (r *Repository) SystemSetting(key string) (*model.SystemSetting, error) {
	var setting model.SystemSetting
	if err := r.db.First(&setting, "key = ?", key).Error; err != nil {
		return nil, err
	}
	return &setting, nil
}

func (r *Repository) SystemSettingOptional(key string) (*model.SystemSetting, error) {
	var setting model.SystemSetting
	if err := r.db.Where("key = ?", key).Limit(1).Find(&setting).Error; err != nil {
		return nil, err
	}
	if setting.Key == "" {
		return nil, nil
	}
	return &setting, nil
}

func (r *Repository) SaveSystemSetting(setting *model.SystemSetting) error {
	return r.db.Save(setting).Error
}

func (r *Repository) SystemSettingsWithPrefix(prefix string, limit int) ([]model.SystemSetting, error) {
	var settings []model.SystemSetting
	escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(prefix)
	err := r.db.Where(`key LIKE ? ESCAPE '\'`, escaped+"%").Order("updated_at desc").Limit(limit).Find(&settings).Error
	return settings, err
}

func (r *Repository) SaveSystemSettings(settings ...*model.SystemSetting) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		for _, setting := range settings {
			if err := tx.Save(setting).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *Repository) ArkPrivateAssetBinding(resourceID string, projectName string) (*model.ArkPrivateAssetBinding, error) {
	var binding model.ArkPrivateAssetBinding
	if err := r.db.Where("resource_id = ? AND project_name = ?", resourceID, projectName).First(&binding).Error; err != nil {
		return nil, err
	}
	return &binding, nil
}

// CreateArkPrivateAssetBinding establishes a single uploader for a resource
// and Ark Project. Other workers can wait for that binding instead of
// importing the same image repeatedly.
func (r *Repository) CreateArkPrivateAssetBinding(binding *model.ArkPrivateAssetBinding) (bool, error) {
	result := r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "resource_id"}, {Name: "project_name"}},
		DoNothing: true,
	}).Create(binding)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

func (r *Repository) SaveArkPrivateAssetBinding(binding *model.ArkPrivateAssetBinding) error {
	return r.db.Save(binding).Error
}

func (r *Repository) DeleteSystemSetting(key string) error {
	return r.db.Delete(&model.SystemSetting{}, "key = ?", key).Error
}
