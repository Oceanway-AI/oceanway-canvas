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

func ecommerceAgentTestRepository(t *testing.T) *Repository {
	t.Helper()
	db, err := testdb.Open(t, &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AutoMigrate(&model.EcommerceProject{}, &model.EcommerceAgentRun{}, &model.EcommerceAgentPage{}, &model.EcommerceAgentCanvasVersion{}); err != nil {
		t.Fatal(err)
	}
	return New(db)
}

func TestLatestEcommerceAgentRunByProjectIsOwnerScoped(t *testing.T) {
	repo := ecommerceAgentTestRepository(t)
	for _, run := range []model.EcommerceAgentRun{
		{ID: "older", UserID: "owner", ProjectID: "project", IdempotencyKey: "older", Status: "completed", CreatedAt: time.Now().Add(-time.Hour)},
		{ID: "latest", UserID: "owner", ProjectID: "project", IdempotencyKey: "latest", Status: "completed", CreatedAt: time.Now()},
		{ID: "other-project", UserID: "owner", ProjectID: "elsewhere", IdempotencyKey: "elsewhere", Status: "completed", CreatedAt: time.Now().Add(time.Hour)},
	} {
		run.CanvasJSON = "{}"
		if err := repo.db.Create(&run).Error; err != nil {
			t.Fatal(err)
		}
	}
	latest, err := repo.LatestEcommerceAgentRunByProject("owner", "project")
	if err != nil || latest.ID != "latest" {
		t.Fatalf("latest=%+v err=%v", latest, err)
	}
	if _, err := repo.LatestEcommerceAgentRunByProject("another-user", "project"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("cross-account project read: %v", err)
	}
}

func TestEcommerceAgentPartialCompletionAndIdempotency(t *testing.T) {
	repo := ecommerceAgentTestRepository(t)
	run := &model.EcommerceAgentRun{ID: "run-1", UserID: "owner", IdempotencyKey: "same", ProjectID: "project-1", Status: "running", Mode: "mock", PageCount: 10}
	project := &model.EcommerceProject{ID: "project-1", UserID: "owner", Name: "商品", Status: "draft", DraftJSON: "{}", PlanJSON: "{}", Revision: 1}
	if err := repo.CreateEcommerceAgentRun(run, project); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 3; i++ {
		if err := repo.SaveEcommerceAgentPage("owner", &model.EcommerceAgentPage{ID: fmt.Sprintf("page-%d", i), RunID: run.ID, Index: i, Status: "completed", URL: "data:image/svg+xml,..."}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := repo.FinishEcommerceAgentRun("owner", run.ID, "completed", ""); err != nil {
		t.Fatal(err)
	}
	finished, err := repo.EcommerceAgentRun("owner", run.ID)
	if err != nil || finished.Status != "completed" || finished.SuccessPages != 3 {
		t.Fatalf("completed run=%+v err=%v", finished, err)
	}
	if _, err := repo.FinishEcommerceAgentRun("owner", run.ID, "completed", ""); err != nil {
		t.Fatal(err)
	}
	duplicate := *run
	duplicate.ID = "run-2"
	if err := repo.CreateEcommerceAgentRun(&duplicate, nil); err == nil {
		t.Fatal("duplicate idempotency key accepted")
	}
	if _, err := repo.EcommerceAgentRun("other", run.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("other user read run: %v", err)
	}
}

func TestEcommerceAgentZeroSuccessCancellation(t *testing.T) {
	repo := ecommerceAgentTestRepository(t)
	run := &model.EcommerceAgentRun{ID: "run-3", UserID: "owner", IdempotencyKey: "zero", ProjectID: "project-3", Status: "running", PageCount: 5}
	project := &model.EcommerceProject{ID: "project-3", UserID: "owner", Name: "商品", Status: "draft", DraftJSON: "{}", PlanJSON: "{}", Revision: 1}
	if err := repo.CreateEcommerceAgentRun(run, project); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.FinishEcommerceAgentRun("owner", run.ID, "cancelled", "CANCELLED"); err != nil {
		t.Fatal(err)
	}
	finished, err := repo.EcommerceAgentRun("owner", run.ID)
	if err != nil || finished.Status != "cancelled" || finished.SuccessPages != 0 || finished.FinishedAt == nil {
		t.Fatalf("cancelled run=%+v err=%v", finished, err)
	}
}
