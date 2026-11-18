package database

import (
	"yingce/backend/internal/testdb"

	"testing"
	"yingce/backend/internal/model"

	"gorm.io/gorm"
)

func TestStudioCreationSubmissionIsOptionalAndUnique(t *testing.T) {
	db, err := testdb.Open(t, &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err = MigrateStudioSchema(db); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"manual-one", "manual-two"} {
		if err = db.Create(&model.Task{ID: id}).Error; err != nil {
			t.Fatal(err)
		}
	}
	sid := "submission"
	if err = db.Create(&model.Task{ID: "creation-one", CreationSubmissionID: &sid}).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Create(&model.Task{ID: "creation-two", CreationSubmissionID: &sid}).Error; err == nil {
		t.Fatal("duplicate execution admitted")
	}
	if !db.Migrator().HasTable(&model.CreationRun{}) || !db.Migrator().HasTable(&model.CreationSubmission{}) {
		t.Fatal("creation tables missing")
	}
}
