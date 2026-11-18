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
	for _, table := range []any{&model.User{}, "user_identities", "auth_sessions", &model.ModelChannel{}, "billing_orders", "payment_orders", "payment_provider_configs", "sms_channels", "user_groups", "user_storage_grants", &model.PluginPlatformState{}, &model.AnnouncementImageDraft{}} {
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

func studioSchema98DB(t *testing.T) *gorm.DB {
	t.Helper()
	db := studioSchemaDB(t)
	if err := MigrateStudioSchema(db); err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(&schemaMigration{}, "version >= 99").Error; err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"resources", "resource_deletion_jobs"} {
		if err := db.Exec("ALTER TABLE " + table + " ADD COLUMN endpoint text, ADD COLUMN bucket varchar(160), ADD COLUMN storage_setting_id varchar(36)").Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Exec("ALTER TABLE ecommerce_agent_runs ADD COLUMN billing_order_id text, ADD COLUMN unit_microcredits bigint").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("ALTER TABLE drama_productions ADD COLUMN budget_microcredits bigint, ADD COLUMN spent_microcredits bigint").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("ALTER TABLE drama_jobs ADD COLUMN cost_microcredits bigint").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("ALTER TABLE tasks ADD COLUMN billing_order_id text, ADD COLUMN authorized_charge_microcredits bigint").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("ALTER TABLE api_call_logs ADD COLUMN billing_order_id text").Error; err != nil {
		t.Fatal(err)
	}
	return db
}

func TestStudioSchema98StorageUpgradePreservesCreativeData(t *testing.T) {
	db := studioSchema98DB(t)
	run := model.EcommerceAgentRun{ID: "creative-run", UserID: "101", IdempotencyKey: "creative-run", Status: "done", PageCount: 3, SuccessPages: 2, CanvasJSON: "{}"}
	if err := db.Create(&run).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("UPDATE ecommerce_agent_runs SET billing_order_id = 'retired-order', unit_microcredits = 42000").Error; err != nil {
		t.Fatal(err)
	}
	production := model.DramaProduction{ProjectID: "creative-drama", UserID: "101", Status: model.DramaStatusRunning, SettingsJSON: "{}"}
	if err := db.Create(&production).Error; err != nil {
		t.Fatal(err)
	}
	dramaJob := model.DramaJob{ID: "creative-job", ProjectID: production.ProjectID, TaskID: "original-task", Status: model.DramaJobRunning, AttemptsJSON: `[{"taskId":"prior-task","status":"succeeded"}]`}
	if err := db.Create(&dramaJob).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("UPDATE drama_productions SET budget_microcredits = 100, spent_microcredits = 40").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("UPDATE drama_jobs SET cost_microcredits = 40").Error; err != nil {
		t.Fatal(err)
	}
	resource := model.Resource{ID: "platform-file", UserID: "101", Provider: "platform", Status: model.ResourceStatusReady, Size: 42}
	if err := db.Create(&resource).Error; err != nil {
		t.Fatal(err)
	}
	job := model.ResourceDeletionJob{ID: "local-cleanup", ResourceID: "system-logo", Provider: "local", ObjectKey: "system/logo.png", Status: model.ResourceDeletionStatusPending}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	if err := RequireStudioSchemaVersion(db); err == nil {
		t.Fatal("started new binary before storage migration")
	}
	var wg sync.WaitGroup
	for range 3 {
		wg.Go(func() {
			if err := MigrateStudioSchema(db); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if err := RequireStudioSchemaVersion(db); err != nil {
		t.Fatal(err)
	}
	var storedRun model.EcommerceAgentRun
	if err := db.First(&storedRun, "id = ?", run.ID).Error; err != nil || storedRun.Status != run.Status || storedRun.SuccessPages != run.SuccessPages || storedRun.IdempotencyKey != run.IdempotencyKey {
		t.Fatalf("changed creative run: %#v, %v", storedRun, err)
	}
	var storedProduction model.DramaProduction
	if err := db.First(&storedProduction, "project_id = ?", production.ProjectID).Error; err != nil || storedProduction.Status != production.Status || storedProduction.UserID != production.UserID {
		t.Fatalf("changed drama production: %#v, %v", storedProduction, err)
	}
	var storedDramaJob model.DramaJob
	if err := db.First(&storedDramaJob, "id = ?", dramaJob.ID).Error; err != nil || storedDramaJob.TaskID != dramaJob.TaskID || storedDramaJob.AttemptsJSON != dramaJob.AttemptsJSON {
		t.Fatalf("changed drama job recovery: %#v, %v", storedDramaJob, err)
	}
	var stored model.Resource
	if err := db.First(&stored, "id = ?", resource.ID).Error; err != nil || stored.UserID != resource.UserID || stored.Size != resource.Size {
		t.Fatalf("changed adopted material: %#v, %v", stored, err)
	}
	var storedJob model.ResourceDeletionJob
	if err := db.First(&storedJob, "id = ?", job.ID).Error; err != nil || storedJob.ObjectKey != job.ObjectKey || storedJob.Status != job.Status {
		t.Fatalf("changed local cleanup: %#v, %v", storedJob, err)
	}
}

func TestStudioSchema98StorageUpgradeRollsBackAndResumes(t *testing.T) {
	db := studioSchema98DB(t)
	// A dependency on the last table interrupts DDL after earlier columns drop.
	if err := db.Exec("CREATE VIEW retained_storage AS SELECT bucket FROM resource_deletion_jobs").Error; err != nil {
		t.Fatal(err)
	}
	if err := MigrateStudioSchema(db); err == nil {
		t.Fatal("expected dependency failure")
	}
	status, err := ReadStudioSchemaStatus(db)
	if err != nil || status.Current != 98 || status.Ready || !db.Migrator().HasColumn("resources", "endpoint") {
		t.Fatalf("partial upgrade escaped transaction: %#v, %v", status, err)
	}
	if err := db.Exec("DROP VIEW retained_storage").Error; err != nil {
		t.Fatal(err)
	}
	if err := MigrateStudioSchema(db); err != nil {
		t.Fatal(err)
	}
}

func TestStudioSchema98StorageUpgradeRejectsUnreleasedObjects(t *testing.T) {
	for _, table := range []string{"resources", "resource_deletion_jobs"} {
		for _, column := range []string{"provider", "endpoint", "bucket", "storage_setting_id"} {
			t.Run(table+"/"+column, func(t *testing.T) {
				db := studioSchema98DB(t)
				if err := db.Table(table).Create(map[string]any{"id": "legacy", column: "old-storage"}).Error; err != nil {
					t.Fatal(err)
				}
				if err := MigrateStudioSchema(db); err == nil {
					t.Fatal("discarded a historical object location")
				}
				var preserved int64
				if err := db.Table(table).Where(column+" = ?", "old-storage").Count(&preserved).Error; err != nil || preserved != 1 {
					t.Fatalf("lost historical location: %d, %v", preserved, err)
				}
			})
		}
	}
}
