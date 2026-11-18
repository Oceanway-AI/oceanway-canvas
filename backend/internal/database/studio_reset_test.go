package database

import (
	"testing"

	"yingce/backend/internal/model"
)

func TestStudioResetAgentReplayCarrier(t *testing.T) {
	for _, test := range []struct {
		name           string
		status         string
		cleanupPending bool
		operation      string
		wantActive     int64
	}{
		{"completed", "completed", false, "cloud_agent", 0},
		{"rejected", "rejected", false, "cloud_agent", 0},
		{"cancelled", "cancelled", false, "cloud_agent", 0},
		{"failed", "failed", false, "cloud_agent", 0},
		{"waiting approval", "waiting_approval", false, "cloud_agent", 1},
		{"cleanup pending", "cancelled", true, "cloud_agent", 1},
		{"missing execution", "", false, "cloud_agent", 1},
		{"ordinary replay", "completed", false, "canvas_text", 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			db := studioSchemaDB(t)
			if err := db.AutoMigrate(&model.Task{}, &model.CloudAgentExecution{}); err != nil {
				t.Fatal(err)
			}
			if err := db.Create(&model.Task{ID: "replay", UserID: "9", Operation: test.operation, Status: model.TaskStatusTextReplay}).Error; err != nil {
				t.Fatal(err)
			}
			if test.status != "" {
				if err := db.Create(&model.CloudAgentExecution{ID: "replay", UserID: "9", Status: test.status, CleanupPending: test.cleanupPending}).Error; err != nil {
					t.Fatal(err)
				}
			}
			inventory, err := StudioDataInventory(db)
			if err != nil || inventory.ActiveTasks != test.wantActive {
				t.Fatalf("active tasks = %d, want %d, error = %v", inventory.ActiveTasks, test.wantActive, err)
			}
			if err := requireResettableStudioData(inventory); (err != nil) != (test.wantActive > 0) {
				t.Fatalf("unexpected reset eligibility: %v", err)
			}
		})
	}
}
