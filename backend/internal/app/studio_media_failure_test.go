package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"yingce/backend/internal/model"
	"yingce/backend/internal/platformintegration"
)

func TestStudioMediaFailurePreservesAttribution(t *testing.T) {
	for _, category := range []string{"user", "model", "platform", "unknown"} {
		message, raw := studioMediaFailureDiagnostic("original", &platformclient.MediaFailure{Code: "failure", Category: category, Source: "provider", Stage: "generation", Message: "safe reason", Recovery: platformclient.MediaRecoveryAction{Action: "none", Availability: "unavailable"}}, true)
		var d model.TaskExecutionDiagnostic
		if err := json.Unmarshal([]byte(raw), &d); err != nil {
			t.Fatal(err)
		}
		if d.Category != category || d.Source != "provider" || d.DiagnosticID != "original" || !d.TaskSubmitted || d.RecoveryAction != "none" || !strings.Contains(message, "safe reason") {
			t.Fatal(d, message)
		}
		if strings.Contains(message, "退还") || strings.Contains(message, "重新生成") {
			t.Fatal("invented funds or retry authority")
		}
	}
}

func TestStudioMediaOriginalRecoveryUsesPlatformAuthority(t *testing.T) {
	s, db, _, _ := creationTestService(t)
	if err := db.AutoMigrate(&model.StudioExecution{}); err != nil {
		t.Fatal(err)
	}
	_, diagnostic := studioMediaFailureDiagnostic("original", &platformclient.MediaFailure{Code: "material_archive_unavailable", Category: "platform", Source: "platform", Stage: "archive", Message: "保存失败", Recovery: platformclient.MediaRecoveryAction{Action: "recover_original_result", Availability: "available"}}, true)
	task := model.Task{ID: "original", UserID: "user", Provider: "platform", Status: model.TaskStatusRunning, ExecutionDiagnosticJSON: diagnostic}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	auth, err := s.encryptSettingSecret("exa_original")
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := s.encryptStudioValue(platformclient.MediaRecovery{Version: "oceanway.media-request.v1", RequestID: "studio-original", TaskID: "platform-original", Status: "SUCCESS", TaskAvailable: true})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Create(&model.StudioExecution{TaskID: task.ID, UserID: task.UserID, State: "submitted", AuthorizationEncrypted: auth, ResponseEncrypted: receipt}).Error; err != nil {
		t.Fatal(err)
	}
	posts := 0
	available := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Platform-Execution-Authorization") != "exa_original" {
			t.Error("original grant lost")
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "GET" && r.URL.Path == "/internal/materials/tasks/platform-original/outputs":
			action := "none"
			if available {
				action = "recover_original_result"
			}
			json.NewEncoder(w).Encode(map[string]any{"protocol": "oceanway.materials.v2", "task_id": "platform-original", "execution_state": "succeeded", "material_state": "failed", "outputs": []any{}, "recovery": map[string]string{"action": action, "availability": "available"}})
		case r.Method == "POST" && r.URL.Path == "/internal/materials/tasks/platform-original/recover":
			posts++
			json.NewEncoder(w).Encode(map[string]any{"protocol": "oceanway.materials.v2", "task_id": "platform-original", "queued_outputs": 1, "action": "query_original"})
		default:
			t.Errorf("unexpected model or other request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	defer server.Close()
	env := map[string]string{"STUDIO_PLATFORM_ORIGIN": server.URL, "STUDIO_PLATFORM_PRODUCT_KEY": "fixture.secret", "STUDIO_PUBLIC_ORIGIN": "https://studio.example.test", "STUDIO_PLATFORM_SESSION_HOSTS": "studio.example.test", "STUDIO_PLATFORM_ALLOWED_ORIGINS": "https://studio.example.test"}
	s.studioPlatform, err = platformintegration.NewGatewayFromEnv(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.RecoverTaskMedia("other-user", task.ID); err == nil || posts != 0 {
		t.Fatal("cross-user recovery")
	}
	got, err := s.RecoverTaskMedia("user", task.ID)
	if err != nil || got.ID != task.ID || got.Status != model.TaskStatusRunning || posts != 1 {
		t.Fatal(got, err, posts)
	}
	available = false
	if _, err = s.RecoverTaskMedia("user", task.ID); err == nil || posts != 1 {
		t.Fatal("stale UI capability authorized recovery")
	}
}

func TestStudioMediaArchiveFailureRemainsOriginalRunningTask(t *testing.T) {
	s, db, _, _ := creationTestService(t)
	lease := time.Now().Add(time.Minute)
	task := model.Task{ID: "original-archive", UserID: "user", Provider: "platform", Status: model.TaskStatusRunning, LeaseOwner: "worker", LeaseExpiresAt: &lease}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	failure := &platformclient.MaterialError{Code: "material_archive_unavailable", Message: "保存失败", Category: "platform", Source: "platform", Stage: "archive"}
	if err := s.deferStudioMediaArchive(&task, failure); err != nil {
		t.Fatal(err)
	}
	var got model.Task
	if err := db.First(&got, "id=?", task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.Status != model.TaskStatusRunning || got.NextPollAt == nil || got.Error != "" || !strings.Contains(got.Stage, "生成已完成") || !strings.Contains(got.ExecutionDiagnosticJSON, `"source":"platform"`) {
		t.Fatal(got)
	}
	// A stale worker may not replace the current recovery diagnostic.
	if err := s.deferStudioMediaArchive(&task, &platformclient.MaterialError{Code: "stale"}); err == nil {
		t.Fatal("stale lease accepted")
	}
	var after model.Task
	db.First(&after, "id=?", task.ID)
	if after.ExecutionDiagnosticJSON != got.ExecutionDiagnosticJSON {
		t.Fatal("stale worker overwrote original failure")
	}
}
