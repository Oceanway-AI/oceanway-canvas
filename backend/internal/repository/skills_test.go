package repository

import (
	"testing"
	"yingce/backend/internal/testdb"

	"yingce/backend/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestPublicSkillCategoryCounts(t *testing.T) {
	db, err := testdb.Open(t, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	if err := db.AutoMigrate(&model.Skill{}); err != nil {
		t.Fatal(err)
	}

	skills := []model.Skill{
		{ID: "drama-1", Status: 1, Tag: "drama"},
		{ID: "drama-2", Status: 1, Tag: "drama"},
		{ID: "private-drama", Status: 1, IsPrivate: true, Tag: "drama"},
		{ID: "disabled-ecommerce", Status: 0, Tag: "ecommerce"},
		{ID: "ecommerce-1", Status: 1, Tag: "ecommerce"},
	}
	if err := db.Create(&skills).Error; err != nil {
		t.Fatal(err)
	}

	counts, err := New(db).PublicSkillCategoryCounts()
	if err != nil {
		t.Fatal(err)
	}
	if counts["drama"] != 2 || counts["ecommerce"] != 1 {
		t.Fatalf("public skill category counts = %#v, want drama=2 and ecommerce=1", counts)
	}
}

func TestAdminSkillSearchUsesAuthorSnapshotWithoutLocalAccounts(t *testing.T) {
	db, err := testdb.Open(t, &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Skill{}); err != nil {
		t.Fatal(err)
	}
	skill := model.Skill{ID: "snapshot-skill", OwnerID: "101", AuthorName: "Platform Author", Name: "Example", Status: 1}
	if err := db.Create(&skill).Error; err != nil {
		t.Fatal(err)
	}
	for _, keyword := range []string{"Platform Author", "101"} {
		rows, total, err := New(db).AdminSkills(AdminSkillListFilter{Keyword: keyword, Limit: 20})
		if err != nil || total != 1 || len(rows) != 1 || rows[0].ID != skill.ID {
			t.Fatalf("search %q: %v, %d, %v", keyword, rows, total, err)
		}
	}
	if db.Migrator().HasTable("users") {
		t.Fatal("search recreated local accounts")
	}
}
