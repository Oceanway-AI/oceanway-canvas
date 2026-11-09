//go:build studio_asset_browser

package handler

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"yingce/backend/internal/model"
)

// Manual browser fixture: the real Studio router, SDK, service and migrated
// isolated DB run unchanged. Only Platform and public CC0 media are fixtures.
// Never built into the application; no model/execution upstream is configured.
func TestStudioAssetBrowserFixture(t *testing.T) {
	address := os.Getenv("STUDIO_ASSET_FIXTURE_ADDR")
	if !strings.HasPrefix(address, "127.0.0.1:") {
		t.Fatal("explicit loopback fixture address required")
	}
	f := newStudioFixture(t)
	f.catalog = `{"success":true,"data":{"items":[]}}`
	imageURL := "https://mdn.github.io/shared-assets/images/examples/balloons.jpg"
	videoURL := "https://interactive-examples.mdn.mozilla.net/media/cc0-videos/flower.mp4"
	var mu sync.Mutex
	bindings := map[string]platformclient.Binding{}
	creates, accesses := 0, 0
	material := func(id string) platformclient.Asset {
		a := studioMaterialFixtureAsset()
		a.AssetID = id
		a.Title = "免费图片参考"
		a.CurrentVersionID = "v-" + id
		a.Version = platformclient.AssetVersion{AssetID: id, VersionID: "v-" + id, FileID: "f-" + id}
		a.File.FileID = "f-" + id
		a.File.ByteLength = "31936"
		a.File.ContentType = "image/jpeg"
		if id == "fixture-video" {
			a.Kind = "video"
			a.Title = "免费视频参考"
			a.File.ContentType = "video/mp4"
			a.File.ByteLength = "1128375"
		}
		return a
	}
	f.materials = func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		respond := func(v any) { _ = json.NewEncoder(w).Encode(v) }
		p := strings.TrimPrefix(r.URL.Path, "/api/platform/v1/materials")
		switch {
		case p == "/authorization":
			respond(map[string]any{"authorization": "mat1.fixture.signature", "expires_at": time.Now().Add(time.Hour).Unix()})
		case p == "/storage":
			respond(map[string]any{"limit_bytes": "21474836480", "used_bytes": "1160311", "reserved_bytes": "0", "remaining_bytes": "21473676169", "over_limit": false})
		case p == "/assets":
			items := []platformclient.Asset{}
			for _, id := range []string{"fixture-image", "fixture-video"} {
				a := material(id)
				if r.URL.Query().Get("kind") == "" || r.URL.Query().Get("kind") == a.Kind {
					items = append(items, a)
				}
			}
			respond(map[string]any{"items": items, "total": strconv.Itoa(len(items)), "page": 1, "limit": 50})
		case p == "/access":
			accesses++
			var in platformclient.AccessRequest
			_ = json.NewDecoder(r.Body).Decode(&in)
			results := []platformclient.AccessResult{}
			for _, item := range in.Items {
				u := imageURL
				if item.AssetID == "fixture-video" && (item.Variant == "playback" || item.Variant == "original") {
					u = videoURL
				}
				results = append(results, platformclient.AccessResult{ItemID: item.ItemID, Status: "ready", AssetID: item.AssetID, VersionID: item.VersionID, FileID: "f-" + item.AssetID, RequestedVariant: item.Variant, ActualVariant: item.Variant, URL: u, ExpiresAt: time.Now().Add(time.Hour).UTC().Format(time.RFC3339), RefreshAt: time.Now().Add(30 * time.Minute).UTC().Format(time.RFC3339), Revision: "fixture-1"})
			}
			respond(platformclient.AccessResponse{Protocol: platformclient.MaterialsProtocol, Results: results})
		case p == "/bindings":
			b, ok := bindings[r.URL.Query().Get("command_id")]
			if !ok {
				w.WriteHeader(404)
				return
			}
			respond(b)
		case strings.HasPrefix(p, "/bindings/") && strings.HasSuffix(p, "/confirm"):
			for key, b := range bindings {
				if p == "/bindings/"+b.BindingID+"/confirm" {
					b.State = "active"
					bindings[key] = b
					respond(b)
					return
				}
			}
			w.WriteHeader(404)
		case strings.HasPrefix(p, "/assets/") && strings.HasSuffix(p, "/bindings"):
			var in platformclient.BindingRequest
			_ = json.NewDecoder(r.Body).Decode(&in)
			id := strings.Split(p, "/")[2]
			b := platformclient.Binding{BindingID: "b-" + id, AssetID: id, VersionID: in.VersionID, TargetApp: "studio", TargetType: in.TargetType, TargetID: in.TargetID, Slot: in.Slot, State: "pending"}
			bindings[in.CommandID] = b
			creates++
			respond(b)
		case strings.HasPrefix(p, "/assets/"):
			respond(material(strings.TrimPrefix(p, "/assets/")))
		default:
			t.Errorf("unexpected fixture upstream: %s %s", r.Method, p)
			w.WriteHeader(503)
		}
	}
	var seed map[string]any
	if os.Getenv("STUDIO_ASSET_FIXTURE_HISTORY") == "1" {
		messages := []any{}
		for _, entry := range []struct{ id, kind string }{{"fixture-image", "image"}, {"fixture-video", "video"}} {
			body, _ := json.Marshal(map[string]any{"assetId": entry.id, "versionId": "v-" + entry.id, "width": 640, "height": 427})
			w := f.request(t, "POST", "/api/resources/adopt", "platform_session=alice", string(body), 200)
			var adopted struct {
				Data struct {
					Resource model.Resource `json:"resource"`
				} `json:"data"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &adopted); err != nil || adopted.Data.Resource.ID == "" {
				t.Fatalf("fixture adoption failed: %v", err)
			}
			messages = append(messages, map[string]any{"id": "a2-" + entry.kind, "role": "assistant", "mode": entry.kind, "content": "A2 已有" + entry.kind + "结果", "createdAt": "2026-11-09T00:00:00Z", "status": "done", "resultStorageKeys": []string{"resource:" + adopted.Data.Resource.ID}})
		}
		var owner model.User
		if err := f.db.First(&owner).Error; err != nil {
			t.Fatal(err)
		}
		seed = map[string]any{"scope": owner.ID, "conversations": []any{map[string]any{"id": "a2-history", "title": "A2 免费既有结果", "updatedAt": "2026-11-09T00:00:00Z", "messages": messages}}}
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/__fixture/history" && seed != nil {
			_ = json.NewEncoder(w).Encode(seed)
			return
		}
		if r.URL.Path == "/__fixture/state" {
			var assets, resources, refs, deletions int64
			f.db.Model(&model.Asset{}).Count(&assets)
			f.db.Model(&model.Resource{}).Count(&resources)
			f.db.Model(&model.StudioMaterialReference{}).Count(&refs)
			f.db.Model(&model.ResourceDeletionJob{}).Count(&deletions)
			mu.Lock()
			defer mu.Unlock()
			var canvases []model.CanvasProject
			var nativeAssets []model.Asset
			f.db.Find(&canvases)
			f.db.Find(&nativeAssets)
			_ = json.NewEncoder(w).Encode(map[string]any{"assets": assets, "resources": resources, "references": refs, "bindings_created": creates, "accesses": accesses, "deletion_jobs": deletions, "canvases": canvases, "native_assets": nativeAssets})
			return
		}
		r.Host = "studio.example.test"
		r.Header.Set("Origin", "https://studio.example.test")
		r.Header.Set("Cookie", "platform_session=alice")
		f.router.ServeHTTP(w, r)
	})}
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			t.Error(err)
		}
	}()
	t.Logf("FREE browser fixture at http://%s (isolated DB, no model requests)", listener.Addr())
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	_ = server.Close()
}
