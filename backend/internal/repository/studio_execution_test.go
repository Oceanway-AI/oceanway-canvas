package repository

import (
	"errors"
	"fmt"
	"testing"
	"time"
	"yingce/backend/internal/testdb"

	"gorm.io/gorm"
	"yingce/backend/internal/model"
)

func TestStudioQueueWithFiveRunningTasksAndNoAccountLimit(t *testing.T) {
	db, err := testdb.Open(t, &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Task{}, &model.StudioExecution{}, &model.TaskTextDelta{}); err != nil {
		t.Fatal(err)
	}
	repo := New(db)
	for i := 0; i < 5; i++ {
		old := model.Task{ID: fmt.Sprintf("old-%d", i), UserID: "user", Type: "canvas_text", Status: model.TaskStatusRunning}
		if err := db.Create(&old).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, kind := range []string{"canvas_image", "canvas_video"} {
		task := model.Task{ID: kind, Type: kind, UserID: "user", Status: model.TaskStatusQueued}
		if _, err := repo.CreateStudioExecution(model.StudioExecution{TaskID: kind, UserID: task.UserID, CommandID: kind, State: "authorizing"}); err != nil {
			t.Fatal(err)
		}
		if err := repo.QueueStudioExecution(task, "fixture", 5); !errors.Is(err, ErrActiveTaskLimit) {
			t.Fatalf("explicit positive limit: %v", err)
		}
		if err := repo.QueueStudioExecution(task, "fixture", 0); err != nil {
			t.Fatalf("unlimited %s: %v", kind, err)
		}
	}
	retry := model.Task{ID: "explicit-retry", UserID: "user", Status: model.TaskStatusFailed}
	if err := db.Create(&retry).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := repo.RetryTaskWithActiveLimit("user", &retry, 0); err != nil {
		t.Fatalf("unlimited retry: %v", err)
	}
	if count, err := repo.ActiveTaskCountForUser("user"); err != nil || count != 8 {
		t.Fatalf("active count = %d, err = %v", count, err)
	}
}

func TestStudioExecutionFencesDispatchAndStop(t *testing.T) {
	for _, submitted := range []bool{false, true} {
		t.Run(map[bool]string{false: "stop-before-send", true: "recover-after-send"}[submitted], func(t *testing.T) {
			db, err := testdb.Open(t, &gorm.Config{})
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

func TestStudioMediaReceiptRefreshFencesStalePolls(t *testing.T) {
	db, err := testdb.Open(t, &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Task{}, &model.StudioExecution{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Task{ID: "media", UserID: "user", Provider: "platform", Status: model.TaskStatusQueued}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.StudioExecution{TaskID: "media", UserID: "user", State: "submitted", ResponseEncrypted: "unknown"}).Error; err != nil {
		t.Fatal(err)
	}
	repo := New(db)
	task, err := repo.ClaimNextTask("worker", time.Minute)
	if err != nil || task == nil {
		t.Fatalf("claim: %v", err)
	}
	if err := repo.RefreshStudioExecutionResponse(task, "unknown", "success"); err != nil {
		t.Fatal(err)
	}
	if err := repo.RefreshStudioExecutionResponse(task, "unknown", "stale"); !errors.Is(err, ErrTaskStateConflict) {
		t.Fatalf("stale observation overwrote success: %v", err)
	}
	db.Model(&model.Task{}).Where("id = ?", task.ID).Update("lease_expires_at", time.Now().Add(-time.Second))
	if err := repo.RefreshStudioExecutionResponse(task, "success", "expired"); !errors.Is(err, ErrTaskStateConflict) {
		t.Fatalf("expired lease overwrote success: %v", err)
	}
	execution, err := repo.StudioExecution("user", task.ID)
	if err != nil || execution.ResponseEncrypted != "success" {
		t.Fatalf("original observation lost: %v", err)
	}
}
