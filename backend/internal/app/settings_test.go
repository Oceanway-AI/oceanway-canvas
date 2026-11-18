package app

import (
	"testing"
	"yingce/backend/internal/testdb"

	"yingce/backend/internal/model"
	"yingce/backend/internal/repository"

	"gorm.io/gorm"
)

func TestMissingOSSSettingsReturnNormalizedDefaults(t *testing.T) {
	db, err := testdb.Open(t, &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.SystemSetting{}, &model.UserOSSSetting{}, &model.StorageLocation{}); err != nil {
		t.Fatal(err)
	}
	svc := New(repository.New(db), t.TempDir())
	admin := &model.User{ID: "admin", Status: model.UserStatusActive, PlatformAuthenticated: true, PlatformSessionID: "fixture-session", StudioPermissions: []model.StudioPermission{model.StudioManageCreativeContent, model.StudioDeleteBuiltinSkill, model.StudioManageSkillCategories, model.StudioCurateSkills}}

	platformSetting, err := svc.AdminOSSSetting(admin)
	if err != nil {
		t.Fatal(err)
	}
	assertDefaultOSSSetting(t, platformSetting)

	userSetting, err := svc.UserOSSSetting(&model.User{ID: "user-1"})
	if err != nil {
		t.Fatal(err)
	}
	assertDefaultOSSSetting(t, userSetting)
}

func assertDefaultOSSSetting(t *testing.T, value *PublicOSSSetting) {
	t.Helper()
	if value.Provider != aliyunOSSProvider || value.PathPrefix != defaultOSSPathPrefix || value.S3Preset != "custom" {
		t.Fatalf("default OSS setting = %+v", value)
	}
}
