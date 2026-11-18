package repository

import (
	"testing"

	"yingce/backend/internal/model"
	"yingce/backend/internal/testdb"

	"gorm.io/gorm"
)

func TestCloudAgentReadsOnePostgresSnapshot(t *testing.T) {
	readers := map[string]func(*Repository) (*model.CloudAgentExecution, error){
		"run": func(repo *Repository) (*model.CloudAgentExecution, error) {
			return repo.CloudAgent("user", "run")
		},
		"active task": func(repo *Repository) (*model.CloudAgentExecution, error) {
			return repo.CloudAgentForActiveTask("user", "task")
		},
		"scheduler": func(repo *Repository) (*model.CloudAgentExecution, error) {
			runs, err := repo.ActiveCloudAgentsAfter("", 1)
			if err != nil || len(runs) == 0 {
				return nil, err
			}
			return &runs[0], nil
		},
	}
	for name, read := range readers {
		t.Run(name, func(t *testing.T) {
			db, err := testdb.Open(t)
			if err != nil {
				t.Fatal(err)
			}
			if err := db.AutoMigrate(&model.CloudAgentExecution{}, &model.CloudAgentEventRecord{}, &model.CloudAgentMessageRecord{}); err != nil {
				t.Fatal(err)
			}
			writer, err := testdb.Reopen(t, db)
			if err != nil {
				t.Fatal(err)
			}
			if err := db.Create(&model.CloudAgentExecution{
				ID: "run", UserID: "user", Status: "running", ActiveTaskID: "task", CheckpointVersion: 2,
			}).Error; err != nil {
				t.Fatal(err)
			}
			committed := false
			// Commit a new checkpoint after the reader sees the control row but
			// before it reads journal and transcript rows on its own connection.
			if err := db.Callback().Query().After("gorm:query").Register("test:checkpoint_between_reads", func(tx *gorm.DB) {
				if committed || tx.Statement.Table != "cloud_agent_executions" {
					return
				}
				committed = true
				writeErr := writer.Transaction(func(update *gorm.DB) error {
					if err := update.Model(&model.CloudAgentExecution{}).Where("id = ?", "run").
						Updates(map[string]any{"event_count": 1, "message_count": 1}).Error; err != nil {
						return err
					}
					if err := update.Create(&model.CloudAgentEventRecord{RunID: "run", UserID: "user", Sequence: 1, EventJSON: `{}`}).Error; err != nil {
						return err
					}
					return update.Create(&model.CloudAgentMessageRecord{RunID: "run", UserID: "user", Kind: "canonical", Sequence: 1, MessageJSON: `{}`}).Error
				})
				if writeErr != nil {
					tx.AddError(writeErr)
				}
			}); err != nil {
				t.Fatal(err)
			}
			run, err := read(New(db))
			if err != nil || run == nil {
				t.Fatalf("read run: %+v %v", run, err)
			}
			if !committed || run.EventCount != len(run.Journal) || run.MessageCount != len(run.Transcript) {
				t.Fatalf("mixed checkpoints: committed=%v events=%d/%d messages=%d/%d", committed, run.EventCount, len(run.Journal), run.MessageCount, len(run.Transcript))
			}
			latest, err := read(New(db))
			if err != nil || latest == nil || latest.EventCount != 1 || len(latest.Journal) != 1 || latest.MessageCount != 1 || len(latest.Transcript) != 1 {
				t.Fatalf("next read must see committed checkpoint: %+v %v", latest, err)
			}
		})
	}
}
