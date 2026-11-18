package database

import (
	"errors"
	"fmt"
	"slices"

	"yingce/backend/internal/model"

	"gorm.io/gorm"
)

type StudioResetTable struct {
	Name string `json:"name"`
	Rows int64  `json:"rows"`
}

type StudioResetInventory struct {
	Driver        string             `json:"driver"`
	Database      string             `json:"database"`
	Schema        string             `json:"schema"`
	Tables        []StudioResetTable `json:"tables"`
	UnknownTables []string           `json:"unknownTables"`
	ActiveTasks   int64              `json:"activeTasks"`
	ActiveAgents  int64              `json:"activeAgents"`
	Applied       bool               `json:"applied"`
}

// The journal survives a partial remote release. It contains identifiers only.
type StudioResetRelease struct {
	Kind string `gorm:"primaryKey;size:32"`
	ID   string `gorm:"primaryKey;size:128"`
}

func StudioDataInventory(db *gorm.DB) (StudioResetInventory, error) {
	result := StudioResetInventory{Driver: db.Dialector.Name(), Tables: []StudioResetTable{}, UnknownTables: []string{}}
	if result.Driver != "postgres" {
		return result, errors.New("Studio requires PostgreSQL")
	}
	if err := db.Raw("SELECT current_database(), current_schema()").Row().Scan(&result.Database, &result.Schema); err != nil {
		return result, err
	}
	known := map[string]bool{"schema_migrations": true, "studio_reset_releases": true}
	for _, table := range retiredStudioTables {
		known[table] = true
	}
	for _, row := range StudioModels() {
		statement := &gorm.Statement{DB: db}
		if err := statement.Parse(row); err != nil {
			return result, err
		}
		known[statement.Schema.Table] = true
	}
	tables, err := db.Migrator().GetTables()
	if err != nil {
		return result, err
	}
	slices.Sort(tables)
	for _, table := range tables {
		if !known[table] {
			result.UnknownTables = append(result.UnknownTables, table)
			continue
		}
		item := StudioResetTable{Name: table}
		if err := db.Table(table).Count(&item.Rows).Error; err != nil {
			return result, err
		}
		result.Tables = append(result.Tables, item)
	}
	if db.Migrator().HasTable(&model.Task{}) {
		active := db.Model(&model.Task{}).Where("status NOT IN ?", []string{"succeeded", "failed", "cancelled"})
		if db.Migrator().HasTable(&model.CloudAgentExecution{}) {
			// Pi keeps its root task as a replay carrier after the run ends.
			// Only a matching, fully cleaned execution proves it is inactive.
			finished := db.Model(&model.CloudAgentExecution{}).
				Select("1").Where("id = tasks.id AND user_id = tasks.user_id").
				Where("status IN ? AND cleanup_pending = ?", []string{"completed", "failed", "cancelled", "rejected"}, false)
			active = active.Where("NOT (status = ? AND operation = ? AND EXISTS (?))", model.TaskStatusTextReplay, "cloud_agent", finished)
		}
		if err := active.Count(&result.ActiveTasks).Error; err != nil {
			return result, err
		}
	}
	if db.Migrator().HasTable(&model.CloudAgentExecution{}) {
		if err := db.Model(&model.CloudAgentExecution{}).Where("status NOT IN ? OR cleanup_pending = ?", []string{"completed", "failed", "cancelled", "rejected"}, true).Count(&result.ActiveAgents).Error; err != nil {
			return result, err
		}
	}
	return result, nil
}

// ResetStudioData is an offline maintenance operation. Remote release failures
// commit only their completed journal entries, allowing a later retry to resume.
func ResetStudioData(db *gorm.DB, release func(*gorm.DB) error) error {
	if release == nil {
		return errors.New("Studio reset requires a reference-release implementation")
	}
	inventory, err := StudioDataInventory(db)
	if err != nil {
		return err
	}
	if err := requireResettableStudioData(inventory); err != nil {
		return err
	}
	if err := db.AutoMigrate(&StudioResetRelease{}); err != nil {
		return err
	}
	var releaseErr error
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", postgresSchemaMigrationLockID).Error; err != nil {
			return err
		}
		// All table names came from the fixed model registry.
		for _, table := range append(inventory.Tables, StudioResetTable{Name: "studio_reset_releases"}) {
			if err := tx.Exec(`LOCK TABLE "` + table.Name + `" IN ACCESS EXCLUSIVE MODE`).Error; err != nil {
				return err
			}
		}
		current, err := StudioDataInventory(tx)
		if err != nil {
			return err
		}
		if err := requireResettableStudioData(current); err != nil {
			return err
		}
		if releaseErr = release(tx); releaseErr != nil {
			return nil
		}
		for i := len(current.Tables) - 1; i >= 0; i-- {
			if err := tx.Migrator().DropTable(current.Tables[i].Name); err != nil {
				return err
			}
		}
		return MigrateStudioSchema(tx)
	})
	if err != nil {
		return err
	}
	return releaseErr
}

func requireResettableStudioData(inventory StudioResetInventory) error {
	if len(inventory.UnknownTables) > 0 {
		return fmt.Errorf("数据库包含未登记表，拒绝清理 %v", inventory.UnknownTables)
	}
	if inventory.ActiveTasks > 0 || inventory.ActiveAgents > 0 {
		return errors.New("存在未收尾 Task 或 Agent，请先通过原任务完成或取消并停止 Studio 服务")
	}
	return nil
}
