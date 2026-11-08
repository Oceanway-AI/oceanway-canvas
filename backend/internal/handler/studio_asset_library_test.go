package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"yingce/backend/internal/model"
)

func studioLibraryResource(t *testing.T, f *studioFixture, id string) model.User {
	t.Helper()
	f.request(t, "GET", "/api/auth/session", "platform_session=alice", "", 200)
	var user model.User
	if err := f.db.First(&user).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Create(&model.Resource{ID: id, UserID: user.ID, Provider: "platform", Status: model.ResourceStatusReady, Kind: "image", MimeType: "image/png", Size: 8}).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Create(&model.StudioMaterialReference{ResourceID: id, UserID: user.ID, AssetID: "asset_1", VersionID: "version_1", FileID: "file_1", OwnerID: "101", BindingID: "binding_1"}).Error; err != nil {
		t.Fatal(err)
	}
	return user
}

func studioLibraryAsset(id, kind, storageKey string) map[string]any {
	data := map[string]any{"content": "分镜笔记"}
	if kind == "image" {
		data = map[string]any{"storageKey": storageKey, "dataUrl": "", "width": 32, "height": 32, "bytes": 8, "mimeType": "image/png"}
	}
	return map[string]any{"id": id, "kind": kind, "title": "创作素材", "category": "material", "status": "confirmed", "coverUrl": "", "tags": []string{"场景"}, "createdAt": "2026-11-09T00:00:00Z", "updatedAt": "2026-11-09T00:00:00Z", "data": data}
}

func TestStudioNativeAssetLibraryOrganizeAndRecover(t *testing.T) {
	f := newStudioFixture(t)
	studioLibraryResource(t, f, "library-image")
	w := f.request(t, "POST", "/api/asset-folders", "platform_session=alice", `{"name":"拍摄参考"}`, 200)
	var folder struct {
		Data struct {
			Folder model.AssetFolder `json:"folder"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &folder); err != nil {
		t.Fatal(err)
	}
	if folder.Data.Folder.ID == "" {
		t.Fatal("folder identity missing")
	}
	for _, kind := range []string{"text", "image"} {
		asset := studioLibraryAsset("library-"+kind+"-asset", kind, "resource:library-image")
		asset["folderId"] = folder.Data.Folder.ID
		body, _ := json.Marshal(map[string]any{"asset": asset})
		f.request(t, "PUT", "/api/assets/"+asset["id"].(string), "platform_session=alice", string(body), 200)
	}
	w = f.request(t, "GET", "/api/assets?page=1&pageSize=40&kind=image", "platform_session=alice", "", 200)
	if !strings.Contains(w.Body.String(), `"total":1`) || !strings.Contains(w.Body.String(), "resource:library-image") {
		t.Fatal(w.Body.String())
	}
	f.request(t, "GET", "/api/assets/library-image-asset", "platform_session=bob", "", 404)
	w = f.request(t, "GET", "/api/asset-folders", "platform_session=bob", "", 200)
	if strings.Contains(w.Body.String(), "拍摄参考") {
		t.Fatal("folder leaked")
	}
	f.request(t, "PATCH", "/api/assets/folder", "platform_session=bob", `{"assetIds":["library-image-asset"],"folderId":""}`, 400)
	f.request(t, "PATCH", "/api/asset-folders/"+folder.Data.Folder.ID, "platform_session=alice", `{"name":"新版参考"}`, 200)
	f.request(t, "DELETE", "/api/asset-folders/"+folder.Data.Folder.ID, "platform_session=alice", "", 200)
	for _, status := range []string{"archived", "confirmed"} {
		asset := studioLibraryAsset("library-image-asset", "image", "resource:library-image")
		asset["status"] = status
		asset["title"] = "更新后的标题"
		asset["note"] = "保留文件"
		body, _ := json.Marshal(map[string]any{"asset": asset})
		f.request(t, "PUT", "/api/assets/library-image-asset", "platform_session=alice", string(body), 200)
	}
	// Reopen uses persisted native snapshot, not a browser-only asset.
	w = f.request(t, "GET", "/api/user-data/snapshot", "platform_session=alice", "", 200)
	if !strings.Contains(w.Body.String(), "更新后的标题") || !strings.Contains(w.Body.String(), "保留文件") {
		t.Fatal(w.Body.String())
	}
	var count int64
	f.db.Model(&model.Resource{}).Count(&count)
	if count != 1 {
		t.Fatal("business edits copied or deleted files")
	}
}

func TestStudioNativeAssetLibraryRejectsUnsafeResourcesAndPurge(t *testing.T) {
	f := newStudioFixture(t)
	studioLibraryResource(t, f, "library-image")
	for _, tc := range []struct {
		name, cookie, key string
		code              int
	}{
		{"foreign", "platform_session=bob", "resource:library-image", 404},
		{"missing", "platform_session=alice", "resource:missing", 404},
		{"external", "platform_session=alice", "https://external.test/private.png", 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(map[string]any{"asset": studioLibraryAsset(tc.name, "image", tc.key)})
			f.request(t, "PUT", "/api/assets/"+tc.name, tc.cookie, string(body), tc.code)
		})
	}
	asset := studioLibraryAsset("safe-asset", "image", "resource:library-image")
	body, _ := json.Marshal(map[string]any{"asset": asset})
	f.request(t, "PUT", "/api/assets/safe-asset", "platform_session=alice", string(body), 200)
	f.request(t, "DELETE", "/api/assets/safe-asset", "platform_session=alice", "", 409)
	f.request(t, "POST", "/api/assets/batch-delete", "platform_session=alice", `["safe-asset"]`, 409)
	f.request(t, "GET", "/api/assets/safe-asset", "platform_session=alice", "", 200)
	var count int64
	f.db.Model(&model.ResourceDeletionJob{}).Count(&count)
	if count != 0 {
		t.Fatal("legacy physical deletion scheduled")
	}
}

func TestStudioMaterialBrowseDoesNotAdopt(t *testing.T) {
	f := newStudioFixture(t)
	calls := 0
	f.materials = func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/platform/v1/materials/access" {
			t.Errorf("browse mutated materials: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(500)
			return
		}
		calls++
		var req platformclient.AccessRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		item := req.Items[0]
		if item.BindingID != "" || item.Purpose != "display" || item.Variant != "thumbnail" || item.VersionID != "version_1" {
			t.Error("browse request changed identity/intent")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(platformclient.AccessResponse{Protocol: platformclient.MaterialsProtocol, Results: []platformclient.AccessResult{{ItemID: item.ItemID, Status: "ready", AssetID: item.AssetID, VersionID: item.VersionID, FileID: "file_1", RequestedVariant: "thumbnail", ActualVariant: "thumbnail", URL: "https://cdn.example.test/thumb?sign=private", ExpiresAt: "2099-01-01T01:00:00Z", RefreshAt: "2099-01-01T00:30:00Z", Revision: "1"}}})
	}
	w := f.request(t, "POST", "/api/materials/access", "platform_session=alice", `{"items":[{"item_id":"thumb","asset_id":"asset_1","version_id":"version_1","purpose":"display","variant":"thumbnail"}]}`, 200)
	if !strings.Contains(w.Header().Get("Cache-Control"), "no-store") {
		t.Fatal("signed access response cacheable")
	}
	f.request(t, "POST", "/api/materials/access", "platform_session=alice", `{"items":[{"item_id":"bad","asset_id":"asset_1","version_id":"version_1","purpose":"provider-input","variant":"original"}]}`, 400)
	var resources, refs int64
	f.db.Model(&model.Resource{}).Count(&resources)
	f.db.Model(&model.StudioMaterialReference{}).Count(&refs)
	if calls != 1 || resources != 0 || refs != 0 {
		t.Fatal("browse created permanent adoption")
	}
}
