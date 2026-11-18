package repository

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"yingce/backend/internal/model"
	"yingce/backend/internal/testdb"
)

func TestCloudAgentConversationAdmissionAcrossConnections(t *testing.T) {
	db, err := testdb.Open(t, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	second, err := testdb.Reopen(t, db, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	testCloudAgentConversationAdmission(t, New(db), New(second))
}

func testCloudAgentConversationAdmission(t *testing.T, a, b *Repository) {
	t.Helper()
	if err := a.db.AutoMigrate(&model.Task{}, &model.CloudAgentExecution{}, &model.CanvasProject{}); err != nil {
		t.Fatal(err)
	}
	for _, item := range []any{
		&model.CloudAgentExecution{ID: "parent", UserID: "user", ConversationID: "parent", Status: "completed", Revision: 7},
		&model.CanvasProject{ID: "canvas-a", UserID: "user"},
		&model.CanvasProject{ID: "canvas-b", UserID: "user"},
	} {
		if err := a.db.Create(item).Error; err != nil {
			t.Fatal(err)
		}
	}
	input, _ := json.Marshal(map[string]any{"cloudAgent": map[string]string{"parentId": "parent"}})
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i, repo := range []*Repository{a, b} {
		wg.Add(1)
		go func(i int, repo *Repository) {
			defer wg.Done()
			<-start
			task := &model.Task{ID: fmt.Sprintf("child-%d", i), UserID: "user", ProjectID: fmt.Sprintf("canvas-%c", 'a'+i), Operation: "cloud_agent", Status: model.TaskStatusTextReplay, InputJSON: string(input)}
			results <- repo.CreateTaskWithActiveLimit(task, 100)
		}(i, repo)
	}
	close(start)
	wg.Wait()
	close(results)
	accepted, stale := 0, 0
	for err := range results {
		switch {
		case err == nil:
			accepted++
		case errors.Is(err, ErrCloudAgentStaleParent):
			stale++
		default:
			t.Fatalf("unexpected admission result: %v", err)
		}
	}
	if accepted != 1 || stale != 1 {
		t.Fatalf("same parent branched across canvases: accepted=%d stale=%d", accepted, stale)
	}
	var tasks int64
	if err := a.db.Model(&model.Task{}).Count(&tasks).Error; err != nil || tasks != 1 {
		t.Fatalf("task count=%d %v", tasks, err)
	}
	var parent model.CloudAgentExecution
	if err := a.db.First(&parent, "id = ?", "parent").Error; err != nil || parent.Revision != 7 {
		t.Fatalf("admission changed parent checkpoint: %+v %v", parent, err)
	}
}
