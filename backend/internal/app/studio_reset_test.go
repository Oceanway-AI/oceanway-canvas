package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"yingce/backend/internal/testdb"

	"yingce/backend/internal/database"
	"yingce/backend/internal/model"
	"yingce/backend/internal/platformintegration"

	"github.com/google/uuid"
)

func TestStudioResetResumesReferencesBeforeDroppingData(t *testing.T) {
	dir := t.TempDir()
	db, err := testdb.Open(t)
	if err != nil {
		t.Fatal(err)
	}
	sql, _ := db.DB()
	defer sql.Close()
	if err := database.MigrateStudioSchema(db); err != nil {
		t.Fatal(err)
	}
	failSecond := true
	released := map[string]int{}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("Cookie") != "" {
			t.Error("reset sent browser credentials")
		}
		if r.Method == "DELETE" {
			t.Fatal("reset deleted a shared Platform object")
		}
		if r.URL.Path == "/api/platform/v1/execution-authorizations/exa_fixture/release" {
			if released["a"] != 1 || released["b"] != 1 {
				t.Error("execution released before references")
			}
			released["execution"]++
			w.WriteHeader(204)
			return
		}
		id := strings.TrimPrefix(r.URL.Query().Get("command_id"), "studio-resource-")
		if r.Method == "POST" {
			id = strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/internal/materials/bindings/binding-"), "/release")
			if id == "b" && failSecond {
				w.WriteHeader(503)
				return
			}
			released[id]++
		}
		if id != "a" && id != "b" {
			t.Errorf("unexpected reset request %s", r.URL.Path)
			w.WriteHeader(400)
			return
		}
		state := "active"
		if released[id] > 0 {
			state = "released"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"binding_id": "binding-" + id, "asset_id": "asset-" + id, "version_id": "version-" + id, "target_app": "studio", "target_type": "resource", "target_id": id, "slot": "original", "state": state})
	}))
	defer server.Close()
	env := map[string]string{"STUDIO_PLATFORM_ORIGIN": server.URL, "STUDIO_PLATFORM_PRODUCT_KEY": "fixture.secret", "STUDIO_PUBLIC_ORIGIN": "https://studio.example.test", "STUDIO_PLATFORM_SESSION_HOSTS": "studio.example.test", "STUDIO_PLATFORM_ALLOWED_ORIGINS": "https://studio.example.test"}
	gateway, err := platformintegration.NewGatewayFromEnv(func(key string) string { return env[key] })
	if err != nil {
		t.Fatal(err)
	}
	crypto := &Service{dataDir: dir}
	materialAuth, err := crypto.encryptSettingSecret("mat1.fixture")
	if err != nil {
		t.Fatal(err)
	}
	executionAuth, err := crypto.encryptSettingSecret("exa_fixture")
	if err != nil {
		t.Fatal(err)
	}
	oldOwner := uuid.NewSHA1(uuid.NameSpaceURL, []byte(server.URL+"\n101")).String()
	for _, table := range []string{"users", "user_identities"} {
		if err := db.Exec("CREATE TABLE " + table + " (id text PRIMARY KEY)").Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Table(table).Create(map[string]any{"id": oldOwner}).Error; err != nil {
			t.Fatal(err)
		}
	}
	rows := []any{
		&model.StudioMaterialAuthority{SessionHash: "old-session", UserID: oldOwner, AuthorizationEncrypted: materialAuth, ExpiresAt: time.Now().Add(time.Hour)},
		&model.StudioExecution{TaskID: "task", UserID: oldOwner, CommandID: "command", State: "completed", AuthorizationEncrypted: executionAuth},
		&model.Task{ID: "task", UserID: oldOwner, Status: model.TaskStatusSucceeded},
		&model.CanvasProject{ID: "old-canvas", UserID: oldOwner},
	}
	for _, id := range []string{"a", "b"} {
		rows = append(rows, &model.StudioMaterialReference{ResourceID: id, UserID: oldOwner, OwnerID: "101", AssetID: "asset-" + id, VersionID: "version-" + id, BindingID: "binding-" + id})
	}
	for _, row := range rows {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	before, err := ResetStudioDatabase(context.Background(), db, dir, nil, false)
	if err != nil || before.Applied || calls != 0 {
		t.Fatalf("inventory mutated data: %+v %v", before, err)
	}
	if _, err := ResetStudioDatabase(context.Background(), db, dir, gateway, true); err == nil {
		t.Fatal("partial release accepted")
	}
	var count int64
	if err := db.Model(&model.CanvasProject{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("failed release lost canvas %d %v", count, err)
	}
	if released["a"] != 1 || released["execution"] != 0 {
		t.Fatalf("invalid release order %v", released)
	}
	failSecond = false
	report, err := ResetStudioDatabase(context.Background(), db, dir, gateway, true)
	if err != nil || !report.Applied {
		t.Fatalf("reset failed: %+v %v", report, err)
	}
	if released["a"] != 1 || released["b"] != 1 || released["execution"] != 1 {
		t.Fatalf("retry repeated completed releases %v", released)
	}
	if err := database.RequireStudioSchemaVersion(db); err != nil {
		t.Fatal(err)
	}
	if db.Migrator().HasTable("users") || db.Migrator().HasTable("user_identities") || db.Migrator().HasTable(&database.StudioResetRelease{}) {
		t.Fatal("retired data survived reset")
	}
	if err := db.Model(&model.CanvasProject{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("old canvas survived %d %v", count, err)
	}
}
