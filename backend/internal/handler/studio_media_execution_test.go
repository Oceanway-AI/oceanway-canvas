package handler

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"yingce/backend/internal/model"
)

func TestStudioMediaFreezesQuoteAndRecoversFixedOutput(t *testing.T) {
	f := newStudioFixture(t)
	f.catalog = `{"success":true,"data":{"items":[{"model_name":"exact-image","model_type":"image","invocation_status":"available","price":{"status":"available","version":"price-1"},"media_generation_profile":{"default_variant":"generate","variants":[{"id":"generate","protocol":"openai-images","operation":"generate","method":"POST","path":"/images/generations","defaults":{"n":1,"size":"square"},"specifications_schema":{"type":"object","additionalProperties":false,"properties":{"n":{"type":"integer","minimum":1,"maximum":1},"size":{"enum":["square"]}},"required":["n","size"]},"parameter_bindings":{"n":"n","size":"size"}}]}}]}}`
	var previewBody []byte
	submits, previews, reads, binds := 0, 0, 0, 0
	var binding *platformclient.Binding
	asset := platformclient.Asset{AssetID: "asset_output", Owner: platformclient.MaterialOwner{Kind: "account", ID: "101"}, Kind: "image", State: "ready", Version: platformclient.AssetVersion{AssetID: "asset_output", VersionID: "version_original", FileID: "file_original"}, File: platformclient.AssetFile{FileID: "file_original", SHA256: strings.Repeat("b", 64), ByteLength: "12", ContentType: "image/png", State: "ready"}}
	f.execution = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		respond := func(v any) { _ = json.NewEncoder(w).Encode(v) }
		switch {
		case strings.HasSuffix(r.URL.Path, "/consumption-preview"):
			previews++
			var in platformclient.PreviewRequest
			_ = json.NewDecoder(r.Body).Decode(&in)
			if in.Request.ExpectedPriceVersion != "price-1" || len(in.Request.Specifications) != 0 {
				t.Error("preview lost native authority")
			}
			previewBody = append([]byte(nil), in.Request.NativeRequest...)
			respond(map[string]any{"success": true, "data": platformclient.ConsumptionPreview{Model: "exact-image", ModelType: "image", PriceVersion: "price-1", WalletPoints: "1.23456789", WalletMaxPoints: "1.23456789", Allowed: true, CombinationSupported: true}})
		case r.URL.Path == "/api/platform/v1/execution-authorizations":
			var in map[string]string
			_ = json.NewDecoder(r.Body).Decode(&in)
			respond(map[string]any{"authorization_id": "exa_media", "product": "studio", "task_reference": in["task_reference"], "expires_at": time.Now().Add(time.Hour).Unix()})
		case strings.HasSuffix(r.URL.Path, "/release"):
			w.WriteHeader(204)
		default:
			if r.Header.Get("Cookie") != "" || r.Header.Get("Origin") != "" || r.Header.Get("X-Platform-Execution-Authorization") != "exa_media" {
				t.Error("media worker mixed authority")
			}
			switch {
			case r.URL.Path == "/internal/relay/images/generations":
				submits++
				body, _ := io.ReadAll(r.Body)
				if string(body) != string(previewBody) {
					t.Error("native quote/submit bytes changed")
				}
				raw, _ := base64.RawURLEncoding.DecodeString(r.Header.Get("X-Platform-Media-Consumption"))
				var cap platformclient.MediaConsumptionConstraints
				_ = json.Unmarshal(raw, &cap)
				if cap.WalletMaxPoints != "1.23456789" || cap.ExpectedPriceVersion != "price-1" {
					t.Error("exact quote/ceiling lost")
				}
				w.WriteHeader(503)
			case strings.HasPrefix(r.URL.Path, "/internal/relay/media-requests/"):
				reads++
				respond(map[string]any{"success": true, "data": platformclient.MediaRecovery{RequestID: strings.TrimPrefix(r.URL.Path, "/internal/relay/media-requests/"), TaskID: "platform_original", Status: "SUCCESS", TaskAvailable: true}})
			case strings.HasSuffix(r.URL.Path, "/tasks/platform_original/outputs"):
				respond(platformclient.TaskMaterials{Protocol: platformclient.MaterialsProtocol, TaskID: "platform_original", ExecutionState: "succeeded", FundsState: "settled", MaterialState: "saved", Outputs: []platformclient.OutputState{{Index: 0, MaterialState: "saved", Original: &platformclient.OriginalManifest{Protocol: platformclient.MaterialsProtocol, Owner: asset.Owner, OperationID: "platform_original", Asset: platformclient.AssetReference{AssetID: asset.AssetID, VersionID: asset.Version.VersionID, FileID: asset.File.FileID, SHA256: asset.File.SHA256, ByteLength: asset.File.ByteLength, ContentType: asset.File.ContentType}}}}})
			case strings.HasSuffix(r.URL.Path, "/assets/asset_output"):
				if r.URL.Query().Get("version_id") != "version_original" {
					t.Error("output version not pinned")
				}
				respond(asset)
			case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/bindings"):
				if binding == nil {
					w.WriteHeader(404)
				} else {
					respond(binding)
				}
			case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/bindings"):
				binds++
				var in platformclient.BindingRequest
				_ = json.NewDecoder(r.Body).Decode(&in)
				binding = &platformclient.Binding{BindingID: "binding_output", AssetID: asset.AssetID, VersionID: asset.Version.VersionID, TargetApp: "studio", TargetType: in.TargetType, TargetID: in.TargetID, Slot: in.Slot, State: "pending"}
				w.WriteHeader(503)
			case strings.HasSuffix(r.URL.Path, "/binding_output/confirm"):
				binding.State = "active"
				respond(binding)
			default:
				t.Errorf("unexpected worker request %s %s", r.Method, r.URL.Path)
				w.WriteHeader(500)
			}
		}
	}
	body := `{"commandId":"image-intent","type":"canvas_image","operation":"image","prompt":"cat","model":"exact-image","platform":{"expectedPriceVersion":"price-1","walletMaxPoints":"1.23456789"},"input":{"mode":"image","config":{"channelId":"oceanway-platform","model":"exact-image","imageParameters":{"count":1,"size":"square"}}}}`
	f.request(t, "POST", "/api/tasks/preview", "platform_session=alice", body, 200)
	w := f.request(t, "POST", "/api/tasks", "platform_session=alice", body, 200)
	var created struct {
		Data model.Task `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	for i := 0; i < 3; i++ {
		if err := f.svc.ProcessNextTask(); err != nil {
			t.Fatal(err)
		}
		f.db.Model(&model.Task{}).Where("id = ?", created.Data.ID).Update("next_poll_at", time.Now().Add(-time.Second))
	}
	w = f.request(t, "GET", "/api/tasks/"+created.Data.ID, "platform_session=alice", "", 200)
	if !strings.Contains(w.Body.String(), "succeeded") || !strings.Contains(w.Body.String(), "resource:") {
		t.Fatalf("output not adopted: %s", w.Body.String())
	}
	if submits != 1 || previews != 2 || reads < 1 || binds != 1 {
		t.Fatalf("duplicate lifecycle: submit=%d preview=%d read=%d binds=%d", submits, previews, reads, binds)
	}
	f.request(t, "POST", "/api/tasks", "platform_session=alice", strings.Replace(strings.Replace(body, "image-intent", "invalid-size", 1), "square", "unsupported", 1), 400)
	f.request(t, "POST", "/api/tasks", "platform_session=alice", strings.Replace(strings.Replace(body, "image-intent", "stale-price", 1), "price-1", "old-price", 1), 409)
}
