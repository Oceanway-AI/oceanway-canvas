package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"yingce/backend/internal/model"
)

func studioMaterialFixtureAsset() platformclient.Asset {
	return platformclient.Asset{AssetID: "asset_1", Owner: platformclient.MaterialOwner{Kind: "account", ID: "101"}, Kind: "image", State: "ready", CurrentVersionID: "version_1", Version: platformclient.AssetVersion{AssetID: "asset_1", VersionID: "version_1", FileID: "file_1"}, File: platformclient.AssetFile{FileID: "file_1", SHA256: strings.Repeat("a", 64), ByteLength: "8", ContentType: "image/png", State: "ready"}}
}

func TestStudioMaterialsUploadAdoptRecoverAndAccess(t *testing.T) {
	f := newStudioFixture(t)
	asset := studioMaterialFixtureAsset()
	var creates, completes, bindCreates, confirms, accesses int
	var command string
	var binding *platformclient.Binding
	ready := false
	f.materials = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Add("Set-Cookie", "materials_nonce=next; Secure; HttpOnly")
		respond := func(v any) { _ = json.NewEncoder(w).Encode(v) }
		upload := func() platformclient.UploadSession {
			u := platformclient.UploadSession{Protocol: platformclient.MaterialsProtocol, UploadID: "upload_1", CommandID: command, State: "awaiting_upload", ByteLength: "8", PartSize: "8", Tickets: []platformclient.UploadTicket{{PartNumber: 1, Method: "PUT", URL: "https://objects.example.test/upload?signature=private", Headers: map[string]string{"Content-Type": "image/png"}}}}
			if ready {
				u.State = "ready"
				u.Asset = &asset
				u.Tickets = nil
			}
			return u
		}
		switch r.Method + " " + r.URL.Path {
		case "POST /api/platform/v1/materials/upload-sessions":
			creates++
			var in platformclient.UploadCreate
			_ = json.NewDecoder(r.Body).Decode(&in)
			if in.ByteLength != "8" || in.SHA256 != asset.File.SHA256 {
				t.Error("upload intent changed")
			}
			command = in.CommandID
			respond(upload())
		case "GET /api/platform/v1/materials/upload-sessions/upload_1":
			respond(upload())
		case "POST /api/platform/v1/materials/upload-sessions/upload_1/complete":
			completes++
			ready = true
			// Simulate a committed completion with the response lost. Recovery is GET.
			w.WriteHeader(503)
		case "GET /api/platform/v1/materials/assets/asset_1":
			if r.URL.Query().Get("version_id") != "version_1" {
				t.Error("adoption did not pin version")
			}
			respond(asset)
		case "GET /api/platform/v1/materials/bindings":
			if binding == nil {
				w.WriteHeader(404)
				return
			}
			respond(binding)
		case "POST /api/platform/v1/materials/assets/asset_1/bindings":
			bindCreates++
			var in platformclient.BindingRequest
			_ = json.NewDecoder(r.Body).Decode(&in)
			binding = &platformclient.Binding{BindingID: "binding_1", AssetID: "asset_1", VersionID: "version_1", TargetApp: "studio", TargetType: in.TargetType, TargetID: in.TargetID, Slot: in.Slot, State: "pending"}
			respond(binding)
		case "POST /api/platform/v1/materials/bindings/binding_1/confirm":
			confirms++
			binding.State = "active"
			respond(binding)
		case "POST /api/platform/v1/materials/access":
			accesses++
			var in platformclient.AccessRequest
			_ = json.NewDecoder(r.Body).Decode(&in)
			results := []platformclient.AccessResult{}
			for _, item := range in.Items {
				if item.AssetID != asset.AssetID || item.VersionID != "version_1" || item.BindingID != "binding_1" || item.Purpose != "display" || item.Variant != "preview" {
					t.Error("wrong material access identity/purpose")
				}
				results = append(results, platformclient.AccessResult{ItemID: item.ItemID, Status: "ready", AssetID: item.AssetID, VersionID: item.VersionID, FileID: "file_1", RequestedVariant: item.Variant, ActualVariant: item.Variant, URL: "https://cdn.example.test/preview?signature=private", ExpiresAt: "2099-01-01T01:00:00Z", RefreshAt: "2099-01-01T00:30:00Z", Revision: "2"})
			}
			respond(platformclient.AccessResponse{Protocol: platformclient.MaterialsProtocol, Results: results})
		default:
			t.Errorf("unexpected material request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(500)
		}
	}
	body := `{"commandId":"upload-intent-1","filename":"image.png","contentType":"image/png","byteLength":"8","sha256":"` + asset.File.SHA256 + `"}`
	w := f.request(t, "POST", "/api/resources/platform-uploads", "platform_session=alice", body, 200)
	var created struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	if created.Data.ID == "" || len(w.Header().Values("Set-Cookie")) == 0 {
		t.Fatal("missing upload identity/cookie update")
	}
	f.request(t, "POST", "/api/resources/platform-uploads", "platform_session=alice", strings.Replace(body, `"8"`, `"9"`, 1), 409)
	path := "/api/resources/platform-uploads/" + created.Data.ID
	f.request(t, "GET", path, "platform_session=bob", "", 404)
	f.request(t, "POST", path+"/complete", "platform_session=bob", `{"parts":[]}`, 404)
	f.request(t, "POST", path+"/complete", "platform_session=alice", `{"parts":[{"part_number":1,"etag":"opaque"}]}`, 503)
	w = f.request(t, "GET", path, "platform_session=alice", "", 200)
	var done struct {
		Data struct {
			Resource model.Resource `json:"resource"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &done)
	res := done.Data.Resource
	if res.ID == "" || res.Status != model.ResourceStatusReady || res.Provider != "platform" || res.PublicURL != "" || res.ObjectKey != "" {
		t.Fatalf("invalid adopted resource: %+v", res)
	}
	f.request(t, "POST", "/api/resources/adopt", "platform_session=alice", `{"assetId":"asset_1","versionId":"version_1"}`, 200)
	f.request(t, "POST", "/api/resources/adopt", "platform_session=bob", `{"assetId":"asset_1","versionId":"version_1"}`, 404)
	f.request(t, "GET", "/api/resources/"+res.ID, "platform_session=bob", "", 404)
	w = f.request(t, "POST", "/api/resources/access", "platform_session=alice", `[{"resourceId":"`+res.ID+`","purpose":"display","variant":"original"}]`, 200)
	if !strings.Contains(w.Body.String(), `"actualVariant":"preview"`) {
		t.Fatal("display access silently used original")
	}
	f.request(t, "POST", "/api/resources/access", "platform_session=bob", `[{"resourceId":"`+res.ID+`","purpose":"display","variant":"original"}]`, 200)
	f.request(t, "POST", "/api/resources/access", "platform_session=alice", `[{"resourceId":"`+res.ID+`","purpose":"provider-input","variant":"original"}]`, 200)
	if creates != 1 || completes != 1 || bindCreates != 1 || confirms != 1 || accesses != 1 {
		t.Fatalf("unexpected repeated operations: %d %d %d %d %d", creates, completes, bindCreates, confirms, accesses)
	}
	var count int64
	f.db.Model(&model.Resource{}).Count(&count)
	if count != 1 {
		t.Fatal("duplicate adopted resources")
	}
	var stored model.Resource
	f.db.First(&stored, "id = ?", res.ID)
	if stored.PublicURL != "" || stored.ObjectKey != "" {
		t.Fatal("signed address stored as identity")
	}
}

func TestStudioMaterialFileWithoutOriginUsesSessionBoundWorkerGrant(t *testing.T) {
	f := newStudioFixture(t)
	issues, accesses := 0, 0
	f.materials = func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/platform/v1/materials/authorization" || r.Method != "POST" || r.Header.Get("Origin") != "https://studio.example.test" {
			t.Errorf("unexpected material authorization: %s", r.URL.Path)
			w.WriteHeader(500)
			return
		}
		issues++
		w.Header().Set("Content-Type", "application/json")
		// The published producer sends unix seconds, while rc.5 types this field
		// as a string. The narrow SDK response adapter must preserve this value.
		_ = json.NewEncoder(w).Encode(map[string]any{"authorization": "mat1.fixture.signature", "expires_at": time.Now().Add(time.Hour).Unix()})
	}
	f.execution = func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/materials/access" || r.Method != "POST" || r.Header.Get("Origin") != "" || r.Header.Get("Cookie") != "" || r.Header.Get("X-Platform-Material-Authorization") != "mat1.fixture.signature" {
			t.Errorf("file read fabricated/mixed session authority: %s", r.URL.Path)
			w.WriteHeader(500)
			return
		}
		accesses++
		var in platformclient.AccessRequest
		_ = json.NewDecoder(r.Body).Decode(&in)
		if len(in.Items) != 1 || in.Items[0].BindingID != "binding_retained" || in.Items[0].AssetID != "asset_1" || in.Items[0].VersionID != "version_1" {
			t.Error("fixed binding lost")
		}
		item := in.Items[0]
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(platformclient.AccessResponse{Protocol: platformclient.MaterialsProtocol, Results: []platformclient.AccessResult{{ItemID: item.ItemID, Status: "ready", AssetID: item.AssetID, VersionID: item.VersionID, FileID: "file_1", RequestedVariant: item.Variant, ActualVariant: item.Variant, URL: "https://cdn.example.test/video?signature=private", ExpiresAt: "2099-01-01T01:00:00Z", RefreshAt: "2099-01-01T00:30:00Z", Revision: "fixed-binding"}}})
	}
	// A session established before the materials bootstrap still has a valid
	// identity, but GET must not silently mint a delegated credential.
	f.request(t, "GET", "/api/auth/session", "platform_session=alice", "", 200)
	if issues != 0 {
		t.Fatal("identity read issued a material grant")
	}
	var user model.User
	if err := f.db.First(&user).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Create(&model.Resource{ID: "resource_file", UserID: user.ID, Kind: "video", Provider: "platform", Status: model.ResourceStatusReady}).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Create(&model.StudioMaterialReference{ResourceID: "resource_file", UserID: user.ID, AssetID: "asset_1", VersionID: "version_1", FileID: "file_1", OwnerID: "101", BindingID: "binding_retained"}).Error; err != nil {
		t.Fatal(err)
	}
	read := func(cookie, origin string, status int) {
		r := httptest.NewRequest("GET", "https://studio.example.test/api/resources/resource_file/file", nil)
		r.Header.Set("Cookie", cookie)
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		f.router.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("file status=%d body=%s", w.Code, w.Body.String())
		}
		if status == 307 && !strings.HasPrefix(w.Header().Get("Location"), "https://cdn.example.test/") {
			t.Fatal("missing signed redirect")
		}
	}
	read("platform_session=alice", "", 409)
	f.request(t, "POST", "/api/auth/session", "platform_session=alice", `{}`, 200)
	var stored model.StudioMaterialAuthority
	if err := f.db.First(&stored).Error; err != nil || len(stored.SessionHash) != 64 || stored.AuthorizationEncrypted == "" || strings.Contains(stored.AuthorizationEncrypted, "mat1.") {
		t.Fatal("material authority must be encrypted and indexed by a session digest")
	}
	read("platform_session=alice", "", 307)
	f.request(t, "POST", "/api/auth/session", "platform_session=alice; nonce=next", `{}`, 200)
	read("platform_session=alice; nonce=next", "", 307)
	if issues != 1 {
		t.Fatal("reopening an initialized session reissued a valid grant")
	}
	if err := f.db.Model(&stored).Update("expires_at", time.Now().Add(-time.Minute)).Error; err != nil {
		t.Fatal(err)
	}
	read("platform_session=alice", "", 409)
	f.request(t, "POST", "/api/auth/session", "platform_session=alice", `{}`, 200)
	read("platform_session=alice", "", 307)
	read("platform_session=alice-other-session", "", 409)
	read("platform_session=alice", "https://evil.test", 403)
	read("platform_session=bob", "", 404)
	f.revoked = true
	read("platform_session=alice", "", 401)
	if issues != 2 || accesses != 3 {
		t.Fatalf("grant/access crossed account or repeated: %d %d", issues, accesses)
	}
}

func TestStudioCanvasAcceptsOnlyOwnedBoundPlatformResources(t *testing.T) {
	f := newStudioFixture(t)
	f.request(t, "GET", "/api/auth/session", "platform_session=alice", "", 200)
	var user model.User
	f.db.First(&user)
	if err := f.db.Create(&model.Resource{ID: "bound-resource", UserID: user.ID, Provider: "platform", Status: model.ResourceStatusReady, Kind: "video"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Create(&model.StudioMaterialReference{ResourceID: "bound-resource", UserID: user.ID, AssetID: "asset_1", VersionID: "version_1", FileID: "file_1", OwnerID: "101", BindingID: "binding_1"}).Error; err != nil {
		t.Fatal(err)
	}
	body := `{"mutationId":"with-platform-video","operations":[{"type":"create_canvas","id":"material-canvas","patch":{"title":"素材画布"}},{"type":"create_node","node":{"id":"video-node","type":"video","metadata":{"storageKey":"resource:bound-resource","content":"/api/resources/bound-resource/file"}}}]}`
	f.request(t, "PATCH", "/api/canvas-projects/material-canvas/operations", "platform_session=alice", body, 200)
	read := f.request(t, "GET", "/api/canvas-projects/material-canvas", "platform_session=alice", "", 200)
	if !strings.Contains(read.Body.String(), "bound-resource") {
		t.Fatal("material reference not persisted")
	}
	f.request(t, "PATCH", "/api/canvas-projects/material-canvas/operations", "platform_session=bob", body, 409)
	f.db.Model(&model.StudioMaterialReference{}).Where("resource_id = ?", "bound-resource").Update("binding_id", "")
	missing := strings.ReplaceAll(strings.ReplaceAll(body, "material-canvas", "missing-binding-canvas"), "with-platform-video", "missing-binding")
	f.request(t, "PATCH", "/api/canvas-projects/missing-binding-canvas/operations", "platform_session=alice", missing, 409)
}

func TestStudioMaterialBindingRecoveryAndImmutableVersion(t *testing.T) {
	f := newStudioFixture(t)
	a := studioMaterialFixtureAsset()
	var b *platformclient.Binding
	var command string
	var creates, confirms int
	f.materials = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		respond := func(v any) { _ = json.NewEncoder(w).Encode(v) }
		switch r.Method + " " + r.URL.Path {
		case "GET /api/platform/v1/materials/assets/asset_1":
			respond(a)
		case "GET /api/platform/v1/materials/bindings":
			if b == nil {
				w.WriteHeader(404)
				return
			}
			if r.URL.Query().Get("command_id") != command {
				t.Error("binding recovery changed command")
			}
			respond(b)
		case "POST /api/platform/v1/materials/assets/asset_1/bindings":
			creates++
			var in platformclient.BindingRequest
			_ = json.NewDecoder(r.Body).Decode(&in)
			command = in.CommandID
			b = &platformclient.Binding{BindingID: "binding_1", AssetID: a.AssetID, VersionID: a.Version.VersionID, TargetApp: "studio", TargetType: in.TargetType, TargetID: in.TargetID, Slot: in.Slot, State: "pending"}
			w.WriteHeader(503)
		case "POST /api/platform/v1/materials/bindings/binding_1/confirm":
			confirms++
			b.State = "active"
			respond(b)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(500)
		}
	}
	body := `{"assetId":"asset_1","versionId":"version_1"}`
	f.request(t, "POST", "/api/resources/adopt", "platform_session=alice", body, 503)
	var pending model.Resource
	f.db.First(&pending)
	if pending.Status != model.ResourceStatusPending {
		t.Fatal("ambiguous binding became ready")
	}
	f.request(t, "GET", "/api/resources/"+pending.ID, "platform_session=alice", "", 409)
	f.request(t, "POST", "/api/resources/adopt", "platform_session=alice", body, 200)
	if creates != 1 || confirms != 1 {
		t.Fatal("binding was recreated instead of recovered")
	}
	a.Version.FileID = "other_file"
	a.File.FileID = "other_file"
	f.request(t, "POST", "/api/resources/adopt", "platform_session=alice", body, 500)
	a = studioMaterialFixtureAsset()
	a.Version.VersionID = "version_2"
	f.request(t, "POST", "/api/resources/adopt", "platform_session=alice", body, 502)
	a = studioMaterialFixtureAsset()
	a.Owner.Kind = "workspace"
	f.request(t, "POST", "/api/resources/adopt", "platform_session=alice", body, 404)
	var count int64
	f.db.Model(&model.Resource{}).Count(&count)
	if count != 1 {
		t.Fatal("failed identity created another resource")
	}
}
