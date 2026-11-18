package database

import (
	"errors"
	"fmt"
	"time"

	"yingce/backend/internal/model"

	"gorm.io/gorm"
)

// The Studio baseline never replays the retired account and commercial migrations.
const CurrentStudioSchemaVersion int64 = 98

var studioBaseline = migration{version: CurrentStudioSchemaVersion, name: "studio_platform_identity", checksum: "sha256:studio-platform-identity-v98-20261118"}

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
		row := schemaMigration{Version: studioBaseline.version, Name: studioBaseline.name, Checksum: studioBaseline.checksum, AppliedAt: time.Now().UTC()}
		return tx.Create(&row).Error
	})
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
	if status.Current != status.Expected {
		return status, nil
	}
	var count int64
	if err := db.Model(&schemaMigration{}).Count(&count).Error; err != nil {
		return status, err
	}
	if count != 1 {
		return status, errors.New("Studio 基线包含旧迁移记录，必须完成显式清理")
	}
	var row schemaMigration
	if err := db.First(&row, "version = ?", CurrentStudioSchemaVersion).Error; err != nil {
		return status, err
	}
	if err := validateMigrationRecord(row, studioBaseline); err != nil {
		return status, err
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
	status.Ready = true
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
