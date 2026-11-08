package repository

import (
	"errors"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"yingce/backend/internal/model"
)

func TestStudioExecutionFencesDispatchAndStop(t *testing.T) {
	for _, submitted := range []bool{false, true} {
		t.Run(map[bool]string{false: "stop-before-send", true: "recover-after-send"}[submitted], func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			if err := db.AutoMigrate(&model.Task{}, &model.StudioExecution{}); err != nil {
				t.Fatal(err)
			}
			if err := db.Create(&model.Task{ID: "step", UserID: "user", AgentRunID: "run", Provider: "platform", Status: model.TaskStatusQueued}).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Create(&model.StudioExecution{TaskID: "step", UserID: "user", CommandID: "command", State: "prepared", Released: true}).Error; err != nil {
				t.Fatal(err)
			}
			repo := New(db)
			stale, err := repo.ClaimNextTask("old", time.Minute)
			if err != nil || stale == nil {
				t.Fatalf("claim: %v", err)
			}
			db.Model(&model.Task{}).Where("id = ?", "step").Update("lease_expires_at", time.Now().Add(-time.Second))
			if sent, err := repo.SubmitStudioExecution(stale); sent || !errors.Is(err, ErrTaskStateConflict) {
				t.Fatalf("expired worker dispatched: %v %v", sent, err)
			}
			live, err := repo.ClaimNextTask("new", time.Minute)
			if err != nil || live == nil {
				t.Fatalf("reclaim: %v", err)
			}
			if sent, err := repo.SubmitStudioExecution(stale); sent || !errors.Is(err, ErrTaskStateConflict) {
				t.Fatalf("old worker dispatched: %v %v", sent, err)
			}
			if submitted {
				if sent, err := repo.SubmitStudioExecution(live); !sent || err != nil {
					t.Fatalf("submit: %v %v", sent, err)
				}
				if sent, err := repo.SubmitStudioExecution(live); sent || err != nil {
					t.Fatalf("resubmitted: %v %v", sent, err)
				}
			}
			if err := repo.StopPreparedStudioAgentTasks("other", "run"); err != nil {
				t.Fatal(err)
			}
			if err := repo.StopPreparedStudioAgentTasks("user", "run"); err != nil {
				t.Fatal(err)
			}
			task, err := repo.Task("step")
			if err != nil {
				t.Fatal(err)
			}
			if submitted {
				if task.Status != model.TaskStatusRunning {
					t.Fatal("submitted request cancelled instead of recovered")
				}
			} else {
				if task.Status != model.TaskStatusCancelled {
					t.Fatal("unsent request not stopped")
				}
				if sent, err := repo.SubmitStudioExecution(live); sent || !errors.Is(err, ErrTaskStateConflict) {
					t.Fatalf("stopped worker dispatched: %v %v", sent, err)
				}
			}
		})
	}
}
