package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"yingce/backend/internal/database"
	"yingce/backend/internal/model"
	"yingce/backend/internal/platformintegration"
)

// ResetStudioDatabase runs only from the offline maintenance command. It needs
// the original encryption key and registered Platform instance for old bindings.
func ResetStudioDatabase(ctx context.Context, db *gorm.DB, dataDir string, gateway *platformintegration.Gateway, apply bool) (database.StudioResetInventory, error) {
	inventory, err := database.StudioDataInventory(db)
	if err != nil || !apply {
		return inventory, err
	}
	if gateway == nil || gateway.Client == nil {
		return inventory, errors.New("清理需要原 Platform 配置")
	}
	svc := &Service{dataDir: dataDir, studioPlatform: gateway}
	err = database.ResetStudioData(db.WithContext(ctx), func(tx *gorm.DB) error {
		return svc.releaseStudioResetData(ctx, tx)
	})
	inventory.Applied = err == nil
	return inventory, err
}

func (s *Service) releaseStudioResetData(ctx context.Context, db *gorm.DB) error {
	var references []model.StudioMaterialReference
	var executions []model.StudioExecution
	if db.Migrator().HasTable(&model.StudioMaterialReference{}) {
		if err := db.Order("resource_id").Find(&references).Error; err != nil {
			return err
		}
	}
	if db.Migrator().HasTable(&model.StudioExecution{}) {
		if err := db.Order("task_id").Find(&executions).Error; err != nil {
			return err
		}
	}
	decrypt := func(value string) (string, error) {
		if !strings.HasPrefix(value, encryptedSettingPrefix) {
			return "", errors.New("清理授权必须为原密文")
		}
		key, err := os.ReadFile(filepath.Join(s.dataDir, ".settings-key"))
		if err != nil || len(key) != 32 {
			return "", errors.New("缺少原 .settings-key，授权保留以便恢复")
		}
		return s.decryptSettingSecret(value)
	}
	for _, ref := range references {
		// UUID derivation is used only to verify a retired database's origin.
		id, err := strconv.Atoi(ref.OwnerID)
		legacy := uuid.NewSHA1(uuid.NameSpaceURL, []byte(s.studioPlatform.Origin()+"\n"+ref.OwnerID)).String()
		if err != nil || id <= 0 || strconv.Itoa(id) != ref.OwnerID || (ref.UserID != ref.OwnerID && ref.UserID != legacy) {
			return fmt.Errorf("素材 %s 的主体与 Platform 环境不匹配", ref.ResourceID)
		}
		done, err := studioResetReleased(db, "resource", ref.ResourceID)
		if err != nil {
			return err
		}
		if done {
			continue
		}
		var authority model.StudioMaterialAuthority
		if !db.Migrator().HasTable(&authority) {
			return fmt.Errorf("素材 %s 缺少可恢复的授权", ref.ResourceID)
		}
		if err := db.Where("user_id = ? AND expires_at > ?", ref.UserID, time.Now()).Order("expires_at DESC").First(&authority).Error; err != nil {
			return fmt.Errorf("素材 %s 需要原用户刷新素材授权后重试", ref.ResourceID)
		}
		auth, err := decrypt(authority.AuthorizationEncrypted)
		if err != nil {
			return err
		}
		materials := s.studioPlatform.Client.WorkerMaterials(platformclient.MaterialContext{Authorization: auth})
		call, cancel := context.WithTimeout(ctx, 15*time.Second)
		// Resolve the stable command even if a previous bind response was lost.
		binding, err := materials.Binding(call, "studio-resource-"+ref.ResourceID)
		cancel()
		var upstream *platformclient.Error
		if errors.As(err, &upstream) && upstream.StatusCode == 404 && ref.BindingID == "" {
			if err := saveStudioResetRelease(db, "resource", ref.ResourceID); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return fmt.Errorf("素材 %s 的绑定读取失败，可重试", ref.ResourceID)
		}
		b := binding.Data
		if b.BindingID == "" || (ref.BindingID != "" && b.BindingID != ref.BindingID) || b.AssetID != ref.AssetID || b.VersionID != ref.VersionID || b.TargetApp != "studio" || b.TargetType != "resource" || b.TargetID != ref.ResourceID || b.Slot != "original" {
			return fmt.Errorf("素材 %s 的绑定响应不匹配", ref.ResourceID)
		}
		if b.State != "released" {
			call, cancel := context.WithTimeout(ctx, 15*time.Second)
			result, err := materials.ReleaseBinding(call, b.BindingID)
			cancel()
			if err != nil || result.Data.BindingID != b.BindingID || result.Data.State != "released" {
				return fmt.Errorf("素材 %s 的绑定释放未确认，可重试", ref.ResourceID)
			}
		}
		if err := saveStudioResetRelease(db, "resource", ref.ResourceID); err != nil {
			return err
		}
	}
	// Bindings must be released before the execution grants used to finish work.
	for _, execution := range executions {
		if execution.Released || execution.AuthorizationEncrypted == "" {
			continue
		}
		done, err := studioResetReleased(db, "execution", execution.TaskID)
		if err != nil {
			return err
		}
		if done {
			continue
		}
		auth, err := decrypt(execution.AuthorizationEncrypted)
		if err != nil {
			return err
		}
		call, cancel := context.WithTimeout(ctx, 15*time.Second)
		_, err = s.studioPlatform.Client.ReleaseExecution(call, auth)
		cancel()
		if err != nil {
			return fmt.Errorf("任务 %s 的执行授权释放失败，可重试", execution.TaskID)
		}
		if err := saveStudioResetRelease(db, "execution", execution.TaskID); err != nil {
			return err
		}
	}
	return nil
}

func studioResetReleased(db *gorm.DB, kind, id string) (bool, error) {
	var count int64
	err := db.Model(&database.StudioResetRelease{}).Where("kind = ? AND id = ?", kind, id).Count(&count).Error
	return count == 1, err
}

func saveStudioResetRelease(db *gorm.DB, kind, id string) error {
	return db.Clauses(clause.OnConflict{DoNothing: true}).Create(&database.StudioResetRelease{Kind: kind, ID: id}).Error
}
