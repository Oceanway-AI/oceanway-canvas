package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"yingce/backend/internal/model"
)

func studioBindPreview(t *testing.T, f *studioFixture, cookie, body string) string {
	t.Helper()
	w := f.request(t, "POST", "/api/tasks/preview", cookie, body, http.StatusOK)
	var preview struct {
		Data struct{ PreviewUserID, PreviewBinding string }
	}
	if err := json.Unmarshal(w.Body.Bytes(), &preview); err != nil || preview.Data.PreviewUserID == "" || preview.Data.PreviewBinding == "" {
		t.Fatal("missing authenticated preview binding")
	}
	var request map[string]any
	if err := json.Unmarshal([]byte(body), &request); err != nil {
		t.Fatal(err)
	}
	options := request["platform"].(map[string]any)
	options["previewUserId"], options["previewBinding"] = preview.Data.PreviewUserID, preview.Data.PreviewBinding
	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestStudioMediaPreviewBindsAccountAndCommandBeforeCreation(t *testing.T) {
	f := newStudioFixture(t)
	f.catalog = studioRevisionCatalog
	previews, issues := 0, 0
	f.execution = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/consumption-preview"):
			previews++
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": platformclient.ConsumptionPreview{CapabilityRevision: strings.Repeat("a", 64), ConnectionRevision: 7, Model: "image-model", ModelType: "image", PriceVersion: "price-1", WalletPoints: "0.123456789", Allowed: true, CombinationSupported: true}})
		case strings.HasSuffix(r.URL.Path, "/execution-authorizations"):
			issues++
			var in map[string]string
			_ = json.NewDecoder(r.Body).Decode(&in)
			_ = json.NewEncoder(w).Encode(map[string]any{"authorization_id": "exa_fixture", "product": "studio", "task_reference": in["task_reference"], "expires_at": time.Now().Add(time.Hour).Unix()})
		default:
			t.Errorf("unexpected provider path: %s", r.URL.Path)
			w.WriteHeader(500)
		}
	}
	alice, bob := "platform_session=alice", "platform_session=bob"
	raw, _ := json.Marshal(studioRevisionBody())
	aliceBody := studioBindPreview(t, f, alice, string(raw))
	bobBody := studioBindPreview(t, f, bob, string(raw))
	owner := func(body string) string {
		var in struct {
			Platform struct{ PreviewUserID string }
		}
		if err := json.Unmarshal([]byte(body), &in); err != nil {
			t.Fatal(err)
		}
		return in.Platform.PreviewUserID
	}
	a, b := owner(aliceBody), owner(bobBody)
	if a == b {
		t.Fatal("test identities coincide")
	}
	for _, invalid := range []string{string(raw), aliceBody, strings.Replace(aliceBody, a, b, 1), strings.Replace(bobBody, "revision-intent", "another-command", 1)} {
		w := f.request(t, "POST", "/api/tasks", bob, invalid, http.StatusConflict)
		if !strings.Contains(w.Body.String(), "studio_preview_account_changed") {
			t.Fatal("missing explicit re-preview reason")
		}
	}
	if f.db.Migrator().HasTable(&model.BillingOrder{}) {
		t.Fatal("Studio created local billing")
	}
	for _, table := range []any{&model.Task{}, &model.StudioExecution{}} {
		var count int64
		if err := f.db.Model(table).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("stale confirmation wrote %T count=%d err=%v", table, count, err)
		}
	}
	if f.catalogCalls != 2 || previews != 2 || issues != 0 {
		t.Fatalf("stale confirmation called provider: previews=%d issues=%d", previews, issues)
	}
	f.request(t, "POST", "/api/tasks", bob, bobBody, 200)
	f.request(t, "POST", "/api/tasks", bob, bobBody, 200)
	// Pre-upgrade callers can still recover the same saved intent.
	f.request(t, "POST", "/api/tasks", bob, string(raw), 200)
	if previews != 3 || issues != 1 {
		t.Fatalf("unexpected valid creation/recovery: previews=%d issues=%d", previews, issues)
	}
	f.request(t, "GET", "/api/tasks/by-command/revision-intent?expectedUserId="+a, bob, "", 409)
	f.request(t, "GET", "/api/tasks/by-command/revision-intent?expectedUserId="+b, bob, "", 200)
}
