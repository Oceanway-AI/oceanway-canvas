package database

import (
	"yingce/backend/internal/testdb"

	"testing"
	"yingce/backend/internal/model"

	"gorm.io/gorm"
)

func TestSkillCurationMigration(t *testing.T) {
	db, err := testdb.Open(t, &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	for i := 0; i < 2; i++ {
		if err := db.Transaction(migrateSkillCuration); err != nil {
			t.Fatal(err)
		}
	}
	var state model.SkillCurationSetting
	if err := db.First(&state, 1).Error; err != nil {
		t.Fatal(err)
	}
	if state.Enabled || state.Revision != 0 {
		t.Fatalf("migration must default off: %#v", state)
	}
	db.Model(&state).Updates(map[string]any{"enabled": true, "revision": 3})
	if err := db.Transaction(migrateSkillCuration); err != nil {
		t.Fatal(err)
	}
	db.First(&state, 1)
	if !state.Enabled || state.Revision != 3 {
		t.Fatal("migration overwrote settings")
	}
}
