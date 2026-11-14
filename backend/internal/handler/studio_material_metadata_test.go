package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"github.com/google/uuid"
	"yingce/backend/internal/model"
)

func TestStudioOriginalFileMetadataReconcilesOwnedResources(t *testing.T) {
	const cookie = "platform_session=alice"
	f := newStudioFixture(t)
	f.request(t, "GET", "/api/auth/session", cookie, "", 200)
	var user model.User
	if err := f.db.First(&user).Error; err != nil {
		t.Fatal(err)
	}
	newAsset := studioMaterialFixtureAsset()
	newAsset.Kind, newAsset.File.ContentType = "video", "video/mp4"
	type originalFile struct {
		platformclient.AssetFile
		Width      int   `json:"width,omitempty"`
		Height     int   `json:"height,omitempty"`
		DurationMs int64 `json:"duration_ms,omitempty"`
	}
	wireAsset := func(a platformclient.Asset, width, height int, duration int64) any {
		return struct {
			platformclient.Asset
			File originalFile `json:"file"`
		}{a, originalFile{a.File, width, height, duration}}
	}
	width, height, duration := 640, 320, int64(4064)
	fileID := newAsset.File.FileID
	extra := studioMaterialFixtureAsset()
	extra.AssetID, extra.Version.AssetID = "unbound", "unbound"
	extra.File.FileID, extra.Version.FileID = "other-file", "other-file"
	id := uuid.NewSHA1(uuid.NameSpaceURL, []byte("studio-material\n"+user.ID+"\nasset_1\nversion_1")).String()
	binding := platformclient.Binding{BindingID: "binding_existing", AssetID: newAsset.AssetID, VersionID: newAsset.Version.VersionID, TargetApp: "studio", TargetType: "resource", TargetID: id, Slot: "original", State: "active"}
	details := 0
	f.materials = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "GET /api/platform/v1/materials/assets/asset_1":
			details++
			if r.URL.Query().Get("version_id") != "version_1" {
				t.Error("resource metadata did not pin the original asset version")
			}
			a := newAsset
			a.File.FileID, a.Version.FileID = fileID, fileID
			_ = json.NewEncoder(w).Encode(wireAsset(a, width, height, duration))
		case "GET /api/platform/v1/materials/bindings":
			_ = json.NewEncoder(w).Encode(binding)
		case "GET /api/platform/v1/materials/assets":
			_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{wireAsset(newAsset, width, height, duration), wireAsset(extra, 1280, 720, 0)}, "total": "2", "page": 1, "limit": 50})
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(500)
		}
	}
	type meta struct {
		Width      int
		Height     int
		DurationMs int64
	}
	check := func(label string, got meta, want meta) {
		t.Helper()
		if got != want {
			t.Errorf("%s metadata %+v, want %+v", label, got, want)
		}
	}
	read := func(method, path, body string) meta {
		t.Helper()
		w := f.request(t, method, path, cookie, body, 200)
		var v struct {
			Data struct {
				Resource struct {
					ID string
					meta
				}
			}
		}
		if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil || v.Data.Resource.ID != id {
			t.Fatalf("resource identity lost: %s", w.Body.String())
		}
		return v.Data.Resource.meta
	}
	stored := func() meta {
		t.Helper()
		var r model.Resource
		if err := f.db.First(&r, "id = ? AND user_id = ?", id, user.ID).Error; err != nil {
			t.Fatal(err)
		}
		return meta{r.Width, r.Height, r.DurationMs}
	}
	reset := func() {
		t.Helper()
		if err := f.db.Model(&model.Resource{}).Where("id = ?", id).Updates(map[string]any{"width": 0, "height": 0, "duration_ms": 0}).Error; err != nil {
			t.Fatal(err)
		}
	}
	adopt := `{"assetId":"asset_1","versionId":"version_1","width":1,"height":2,"durationMs":3}`
	want := meta{640, 320, 4064}
	check("new adoption uses the original file", read("POST", "/api/resources/adopt", adopt), want)
	reset()
	check("legacy direct resource", read("GET", "/api/resources/"+id, ""), want)
	check("direct read persisted", stored(), want)
	reset()
	f.request(t, "GET", "/api/resources/"+id, "platform_session=bob", "", 404)
	beforeDetails := details
	w := f.request(t, "GET", "/api/materials/assets", cookie, "", 200)
	var list struct{ Data struct{ Items []meta } }
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil || len(list.Data.Items) != 2 {
		t.Fatalf("list: %s", w.Body.String())
	}
	check("listed bound file", list.Data.Items[0], want)
	check("listed unbound file", list.Data.Items[1], meta{1280, 720, 0})
	check("list persisted", stored(), want)
	if details != beforeDetails {
		t.Error("listing made per-file Platform requests")
	}
	var n int64
	f.db.Model(&model.Resource{}).Count(&n)
	if n != 1 {
		t.Fatalf("listing created %d resources", n)
	}
	width, height, duration = 0, 0, 0
	check("pending authority preserves stored metadata", read("POST", "/api/resources/adopt", adopt), want)
	reset()
	fileID = "wrong-file"
	width, height, duration = 800, 600, 2000
	f.request(t, "GET", "/api/resources/"+id, cookie, "", 502)
	check("mismatched file never updates", stored(), meta{})
	var ref model.StudioMaterialReference
	if err := f.db.First(&ref, "resource_id = ?", id).Error; err != nil || ref.FileID != "file_1" || ref.BindingID != binding.BindingID || ref.SHA256 != strings.Repeat("a", 64) {
		t.Fatal("original binding changed")
	}
}
