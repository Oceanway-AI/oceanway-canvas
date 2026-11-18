package database

import (
	"fmt"
	"sync"
	"testing"
	"yingce/backend/internal/testdb"

	"yingce/backend/internal/model"

	"gorm.io/gorm"
)

func studioSchemaDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := testdb.Open(t)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestStudioSchema(t *testing.T) {
	db := studioSchemaDB(t)
	if err := MigrateStudioSchema(db); err != nil {
		t.Fatal(err)
	}
	if err := MigrateStudioSchema(db); err != nil {
		t.Fatal(err)
	}
	if err := RequireStudioSchemaVersion(db); err != nil {
		t.Fatal(err)
	}
	for _, table := range StudioModels() {
		if !db.Migrator().HasTable(table) {
			t.Fatalf("missing creative table %T", table)
		}
	}
	for _, table := range []any{&model.User{}, "user_identities", "auth_sessions", &model.ModelChannel{}, &model.CreditAccount{}, &model.BillingOrder{}, "payment_orders", "payment_provider_configs", "sms_channels", &model.UserGroup{}, "user_storage_grants", &model.PluginPlatformState{}, &model.AnnouncementImageDraft{}} {
		if db.Migrator().HasTable(table) {
			t.Fatalf("retired table created %T", table)
		}
	}
	for _, index := range []struct {
		table any
		name  string
	}{
		{&model.Resource{}, "idx_resources_user_upload_key"},
		{&model.ProjectAssetCandidate{}, "idx_project_asset_candidates_pending_identity"},
		{&model.CloudAgentExecution{}, "idx_cloud_agent_canvas_active"},
		{&model.Task{}, "idx_tasks_canvas_agent_reservation"},
	} {
		if !db.Migrator().HasIndex(index.table, index.name) {
			t.Fatalf("missing index %s", index.name)
		}
	}
	// Verify actual uniqueness used by uploads and pending asset adoption.
	key := "upload-1"
	for i, owner := range []string{"101", "202", "101"} {
		err := db.Create(&model.Resource{ID: fmt.Sprint(i), UserID: owner, UploadKey: &key}).Error
		if (i == 2) != (err != nil) {
			t.Fatalf("upload uniqueness %d: %v", i, err)
		}
	}
}

func TestStudioSchemaRejectsUnclearedData(t *testing.T) {
	for _, row := range []any{&model.User{}, &model.CanvasProject{}} {
		t.Run(fmt.Sprintf("%T", row), func(t *testing.T) {
			db := studioSchemaDB(t)
			if err := db.AutoMigrate(row); err != nil {
				t.Fatal(err)
			}
			if err := MigrateStudioSchema(db); err == nil {
				t.Fatal("adopted unversioned legacy database")
			}
			if db.Migrator().HasTable(&model.StudioExecution{}) {
				t.Fatal("rejected migration left new tables")
			}
		})
	}
	db := studioSchemaDB(t)
	if err := db.AutoMigrate(&schemaMigration{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&schemaMigration{Version: 97, Name: "retired", Checksum: "retired"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := MigrateStudioSchema(db); err == nil {
		t.Fatal("adopted legacy migration history")
	}
}

func TestStudioSchemaConcurrentPostgres(t *testing.T) {
	db := studioSchemaDB(t)
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := MigrateStudioSchema(db); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if err := RequireStudioSchemaVersion(db); err != nil {
		t.Fatal(err)
	}
}

func TestStudioSchemaRejectsTamperedBaseline(t *testing.T) {
	for _, change := range []string{"name", "checksum", "retired-table"} {
		t.Run(change, func(t *testing.T) {
			db := studioSchemaDB(t)
			if err := MigrateStudioSchema(db); err != nil {
				t.Fatal(err)
			}
			if change == "retired-table" {
				if err := db.Exec("CREATE TABLE users (id text PRIMARY KEY)").Error; err != nil {
					t.Fatal(err)
				}
			} else if err := db.Model(&schemaMigration{}).Where("version = ?", CurrentStudioSchemaVersion).Update(change, "tampered").Error; err != nil {
				t.Fatal(err)
			}
			if err := RequireStudioSchemaVersion(db); err == nil {
				t.Fatal("accepted a modified Studio baseline")
			}
			if err := MigrateStudioSchema(db); err == nil {
				t.Fatal("migration accepted a modified Studio baseline")
			}
		})
	}
}

func TestStudioSchemaRollsBackFailedInitialization(t *testing.T) {
	db := studioSchemaDB(t)
	// This name collides with a required table after several models have been
	// created, exercising PostgreSQL's transactional DDL rollback.
	if err := db.Exec("CREATE TYPE resources AS ENUM ('blocked')").Error; err != nil {
		t.Fatal(err)
	}
	if err := MigrateStudioSchema(db); err == nil {
		t.Fatal("expected model initialization failure")
	}
	tables, err := db.Migrator().GetTables()
	if err != nil || len(tables) != 0 {
		t.Fatalf("failed migration left tables %v: %v", tables, err)
	}
	if err := db.Exec("DROP TYPE resources").Error; err != nil {
		t.Fatal(err)
	}
	if err := MigrateStudioSchema(db); err != nil {
		t.Fatal(err)
	}
}
