package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"yingce/backend/internal/model"
)

func TestStudioHistoricalWalletConsent(t *testing.T) {
	f := newStudioFixture(t)
	const id = "old-capped-video"
	task := model.Task{ID: id, UserID: "101", Type: "canvas_video", Provider: "platform", Status: model.TaskStatusRunning,
		PlatformGenerationRequest: "original-command", ProviderRequestID: "original-task", PlatformRevision: 1, ResultJSON: `{"fundsState":"reserved"}`}
	if err := f.db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	state, writes := "required", 0
	f.execution = func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "platform_session=alice" || r.Header.Get("X-Platform-Product-Key") != "fixture.secret" {
			t.Fatal("missing original payer and product")
		}
		switch {
		case r.Method == "GET" && r.URL.Path == "/internal/products/v1/media-generations/original-command":
			_ = json.NewEncoder(w).Encode(map[string]any{"request_id": "original-command", "task_id": "original-task", "operation_id": "original-operation", "revision": 1, "execution_state": "succeeded", "funds_state": "reserved", "outputs": []any{}, "final_wallet_consent": map[string]string{"state": state, "total_points": "25.1234"}})
		case r.Method == "POST" && r.URL.Path == "/internal/products/v1/media-generations/original-command/final-wallet-consent":
			var in map[string]string
			if json.NewDecoder(r.Body).Decode(&in) != nil || in["task_id"] != "original-task" || in["operation_id"] != "original-operation" || in["total_points"] != "25.1234" {
				t.Error("unexpected final consent")
			}
			writes++
			state = "recorded"
			w.WriteHeader(204)
		default:
			t.Errorf("unexpected call %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	}
	const url, cookie = "/api/tasks/" + id, "platform_session=alice"
	const exact = `{"totalPoints":"25.1234"}`
	confirm := url + "/final-wallet-consent"
	detail := f.request(t, "GET", url, cookie, "", 200).Body.String()
	f.request(t, "POST", confirm, "platform_session=bob", exact, 404)
	f.request(t, "POST", confirm, cookie, `{"totalPoints":"4e0"}`, 400)
	if !strings.Contains(detail, `"finalWalletConsent":{"state":"required","totalPoints":"25.1234"}`) {
		t.Fatalf("missing authoritative consent from detail %s", detail)
	}
	f.request(t, "POST", confirm, cookie, `{"totalPoints":"0.1234"}`, 409)
	f.request(t, "POST", confirm, cookie, exact, 200)
	f.request(t, "POST", confirm, cookie, exact, 200)
	if writes != 1 || !strings.Contains(f.request(t, "GET", url, cookie, "", 200).Body.String(), `"state":"recorded"`) {
		t.Fatal("confirmation not recorded exactly once")
	}
}
