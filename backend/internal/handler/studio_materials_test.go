package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

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
				if item.AssetID != asset.AssetID || item.VersionID != "version_1" || item.Purpose != "display" || item.Variant != "preview" {
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
