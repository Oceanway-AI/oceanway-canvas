package app

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"yingce/backend/internal/model"
	"yingce/backend/internal/platformintegration"
)

func TestStudioLegacyMediaAuthorizationRejectsMissingFrozenRevisions(t *testing.T) {
	s, db, _, _ := creationTestService(t)
	if err := db.AutoMigrate(&model.StudioExecution{}); err != nil {
		t.Fatal(err)
	}
	req := CreateTaskRequest{CommandID: "legacy-media", Type: "canvas_image", Prompt: "cat"}
	intent, _ := json.Marshal(req)
	id := studioTaskID("user", req.CommandID)
	envelope := studioExecutionEnvelope{Task: model.Task{ID: id, UserID: "user"}, Selection: platformintegration.ModelSelection{Kind: "image"}, Consumption: &platformclient.MediaConsumptionConstraints{ExpectedPriceVersion: "p1", WalletMaxPoints: "1"}}
	encoded, err := s.encryptStudioValue(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.StudioExecution{TaskID: id, UserID: "user", CommandID: req.CommandID, IntentHash: fmt.Sprintf("%x", sha256.Sum256(intent)), EnvelopeEncrypted: encoded, State: "authorizing"}).Error; err != nil {
		t.Fatal(err)
	}
	// No gateway is configured: reading this old durable intent must stop
	// before either preview or IssueExecution can be called.
	_, _, err = s.StudioCreateTask(context.Background(), "user", platformclient.SessionContext{}, req)
	if appErr, ok := err.(*AppError); !ok || appErr.Status != 409 {
		t.Fatalf("legacy intent was not rejected before authorization: %v", err)
	}
	var count int64
	if err := db.Model(&model.Task{}).Where("id = ?", id).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("legacy intent queued a task: %d, %v", count, err)
	}
	row, err := s.repo.StudioExecution("user", id)
	if err != nil || row.State != "authorizing" || row.EnvelopeEncrypted != encoded || row.AuthorizationEncrypted != "" {
		t.Fatalf("legacy frozen intent was rewritten: %v", err)
	}
}
