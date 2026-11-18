package database

import (
	"errors"
	"fmt"
	"time"

	"yingce/backend/internal/model"

	"gorm.io/gorm"
)

// The Studio baseline never replays the retired account and commercial migrations.
const CurrentStudioSchemaVersion int64 = 99

var studioMigrations = []migration{
	{version: 98, name: "studio_platform_identity", checksum: "sha256:studio-platform-identity-v98-20261118"},
	{version: 99, name: "studio_platform_storage", checksum: "sha256:studio-platform-storage-v99-20261118"},
}

func StudioModels() []any {
	return []any{
		&model.CloudAgentExecution{},
		&model.UploadReservation{},
		&model.CloudAgentPiSession{},
		&model.CloudAgentEventRecord{},
		&model.CloudAgentMessageRecord{},
		&model.CloudAgentCanvasMutation{},
		&model.CloudAgentResourceLease{},
		&model.CloudAgentGeminiCache{},
		&model.AgentProfile{},
		&model.AgentLesson{},
		&model.AgentMemorySetting{},
		&model.AgentMemoryProposal{},
		&model.AgentMemoryRevision{},
		&model.AgentMemoryDocument{},
		&model.AgentMemoryDocumentRevision{},
		&model.IDSequence{},
		&model.ApiCallLog{},
		&model.AdminAuditEvent{},
		&model.UserDailyActivity{},
		&model.SystemSetting{},
		&model.Skill{},
		&model.SkillLibraryCategory{},
		&model.SkillVersion{},
		&model.SkillFile{},
		&model.UserSkillState{},
		&model.BuiltinSkillTombstone{},
		&model.Tool{},
		&model.ToolFavorite{},
		&model.Resource{},
		&model.ResourceDeletionJob{},
		&model.Asset{},
		&model.AssetFolder{},
		&model.ProjectAssetLink{},
		&model.ProjectAssetFolder{},
		&model.ProjectAssetCandidate{},
		&model.AssetVersion{},
		&model.AssetRepresentation{},
		&model.VoiceProfile{},
		&model.CharacterVoiceBinding{},
		&model.Project{},
		&model.EcommerceProject{},
		&model.EcommerceKnowledgeSource{},
		&model.EcommerceKnowledgeEntry{},
		&model.EcommerceCreativeRecommendation{},
		&model.EcommerceAgentAnalysis{},
		&model.EcommerceAgentRun{},
		&model.EcommerceAgentPage{},
		&model.EcommerceAgentCanvasVersion{},
		&model.StyleProfile{},
		&model.ProjectUnit{},
		&model.CanvasUnitLink{},
		&model.Shot{},
		&model.ShotRevision{},
		&model.ShotArtifact{},
		&model.ShotAssetReference{},
		&model.WorkflowTemplateVersion{},
		&model.WorkflowInstance{},
		&model.WorkflowStepInstance{},
		&model.WorkflowStepTask{},
		&model.ProductionTaskLink{},
		&model.CanvasProject{},
		&model.CanvasSnapshot{},
		&model.CanvasSnapshotResource{},
		&model.CanvasMutationReceipt{},
		&model.CanvasEditLease{},
		&model.WorkbenchTool{},
		&model.WorkbenchPreset{},
		&model.PoseLibrarySource{},
		&model.PoseLibraryItem{},
		&model.InspirationFeedEntry{},
		&model.CanvasShare{},
		&model.PromptTemplate{},
		&model.UserPromptCustomization{},
		&model.Task{},
		&model.CreationRun{},
		&model.CreationSubmission{},
		&model.TaskTextDelta{},
		&model.TaskLog{},
		&model.Result{},
		&model.DramaProduction{},
		&model.DramaCast{},
		&model.DramaEpisode{},
		&model.DramaShot{},
		&model.DramaTake{},
		&model.DramaSegment{},
		&model.DramaJob{},
		&model.SkillCurationSetting{},
		&model.SkillCurationCategory{},
		&model.SkillCurationAssignment{},
		&model.SkillCurationRoot{},
		&model.SkillCurationRootAssignment{},
		&model.StudioMaterialUpload{},
		&model.StudioMaterialReference{},
		&model.StudioExecution{},
		&model.StudioMaterialAuthority{},
		&model.StudioMediaAction{},
		&model.StudioMediaActionAlias{},
	}
}

func MigrateStudioSchema(db *gorm.DB) error {
	if db.Dialector.Name() != "postgres" {
		return errors.New("Studio requires PostgreSQL")
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", postgresSchemaMigrationLockID).Error; err != nil {
			return err
		}
		if tx.Migrator().HasTable(&schemaMigration{}) {
			var count int64
			if err := tx.Model(&schemaMigration{}).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				status, err := ReadStudioSchemaStatus(tx)
				if err != nil {
					return err
				}
				if status.Current == 98 {
					if err := migrateStudioPlatformStorage(tx); err != nil {
						return err
					}
					if err := recordStudioMigration(tx, studioMigrations[1]); err != nil {
						return err
					}
				}
				return RequireStudioSchemaVersion(tx)
			}
		}
		// An unversioned database can still contain account mappings. Its data
		// must go through the explicit reset and reference-release workflow.
		tables, err := tx.Migrator().GetTables()
		if err != nil {
			return err
		}
		for _, table := range tables {
			if table != "schema_migrations" {
				return errors.New("旧 Studio 数据需要先完成引用释放与显式清理，请使用新数据库或执行清理流程")
			}
		}
		if err := tx.AutoMigrate(&schemaMigration{}); err != nil {
			return err
		}
		if err := tx.AutoMigrate(StudioModels()...); err != nil {
			return err
		}
		for _, initialize := range []func(*gorm.DB) error{initializeStudioIndexes, migrateSkillCuration, migrateSkillCurationRoots, migrateCanvasAgentActivityIndexes} {
			if err := initialize(tx); err != nil {
				return err
			}
		}
		for _, migration := range studioMigrations {
			if err := recordStudioMigration(tx, migration); err != nil {
				return err
			}
		}
		return nil
	})
}

func recordStudioMigration(tx *gorm.DB, applied migration) error {
	return tx.Create(&schemaMigration{Version: applied.version, Name: applied.name, Checksum: applied.checksum, AppliedAt: time.Now().UTC()}).Error
}

func migrateStudioPlatformStorage(tx *gorm.DB) error {
	for _, table := range []string{"resources", "resource_deletion_jobs"} {
		var external int64
		if err := tx.Table(table).Where("COALESCE(provider, '') NOT IN ('', 'local', 'platform')").Count(&external).Error; err != nil {
			return err
		}
		if external > 0 {
			return fmt.Errorf("%s 仍有独立对象存储记录，须先完成原对象清理", table)
		}
		for _, column := range []string{"endpoint", "bucket", "storage_setting_id"} {
			if !tx.Migrator().HasColumn(table, column) {
				continue
			}
			if err := tx.Table(table).Where(column + " <> ''").Count(&external).Error; err != nil {
				return err
			}
			if external > 0 {
				return fmt.Errorf("%s.%s 仍有独立对象存储位置，须先完成原对象清理", table, column)
			}
			if err := tx.Migrator().DropColumn(table, column); err != nil {
				return err
			}
		}
	}
	return nil
}

func ReadStudioSchemaStatus(db *gorm.DB) (SchemaStatus, error) {
	status := SchemaStatus{Expected: CurrentStudioSchemaVersion}
	if db.Dialector.Name() != "postgres" {
		return status, errors.New("Studio requires PostgreSQL")
	}
	if !db.Migrator().HasTable(&schemaMigration{}) {
		return status, nil
	}
	if err := db.Model(&schemaMigration{}).Select("COALESCE(MAX(version), 0)").Scan(&status.Current).Error; err != nil {
		return status, err
	}
	var rows []schemaMigration
	if err := db.Order("version").Find(&rows).Error; err != nil {
		return status, err
	}
	for index, row := range rows {
		if index >= len(studioMigrations) || row.Version != studioMigrations[index].version {
			return status, errors.New("Studio 包含未知或缺失的迁移记录")
		}
		if err := validateMigrationRecord(row, studioMigrations[index]); err != nil {
			return status, err
		}
	}
	tables, err := db.Migrator().GetTables()
	if err != nil {
		return status, err
	}
	retired := make(map[string]bool, len(retiredStudioTables))
	for _, table := range retiredStudioTables {
		retired[table] = true
	}
	for _, table := range tables {
		if retired[table] || table == "studio_reset_releases" {
			return status, fmt.Errorf("Studio 数据库仍包含退出表 %s，必须完成显式清理", table)
		}
	}
	status.Ready = status.Current == status.Expected && len(rows) == len(studioMigrations)
	if status.Ready {
		for _, table := range []string{"resources", "resource_deletion_jobs"} {
			for _, column := range []string{"endpoint", "bucket", "storage_setting_id"} {
				if db.Migrator().HasColumn(table, column) {
					return status, fmt.Errorf("Studio 数据库仍包含退出字段 %s.%s", table, column)
				}
			}
		}
	}
	return status, nil
}

func RequireStudioSchemaVersion(db *gorm.DB) error {
	status, err := ReadStudioSchemaStatus(db)
	if err != nil {
		return err
	}
	if !status.Ready {
		return fmt.Errorf("Studio 数据库结构不匹配，当前 %d，要求 %d。旧身份数据须完成引用释放并显式清理，旧制品不能用于此数据库", status.Current, status.Expected)
	}
	return nil
}
