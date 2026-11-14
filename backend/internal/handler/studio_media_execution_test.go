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
	"yingce/backend/internal/repository"
)

func TestStudioMediaFreezesQuoteAndRecoversFixedOutput(t *testing.T) {
	for _, kind := range []string{"image", "video", "video_const"} {
		t.Run(kind, func(t *testing.T) { studioMediaFreezesQuoteAndRecoversFixedOutput(t, kind) })
	}
}

func TestStudioMediaTaskRetainsReferenceInputs(t *testing.T) {
	studioMediaFreezesQuoteAndRecoversFixedOutput(t, "image_reference")
}

func studioMediaFreezesQuoteAndRecoversFixedOutput(t *testing.T, kind string) {
	withReference := kind == "image_reference"
	if withReference {
		kind = "image"
	}
	fixedResolution := kind == "video_const"
	if fixedResolution {
		kind = "video"
	}
	f := newStudioFixture(t)
	f.catalog = `{"success":true,"data":{"items":[{"model_name":"exact-image","model_type":"image","invocation_status":"available","price":{"status":"available","version":"price-1"},"media_generation_profile":{"default_variant":"generate","variants":[{"id":"generate","protocol":"openai-images","operation":"generate","method":"POST","path":"/images/generations","defaults":{"n":1,"size":"square"},"specifications_schema":{"type":"object","additionalProperties":false,"properties":{"n":{"type":"integer","minimum":1,"maximum":1},"size":{"enum":["square"]}},"required":["n","size"]},"parameter_bindings":{"n":"n","size":"size"}}]}}]}}`
	name, submitPath, contentType := "exact-image", "/internal/relay/images/generations", "image/png"
	if kind == "video" {
		name, submitPath, contentType = "doubao-seedance-2-0-fast-260128", "/internal/relay/doubao/api/v3/contents/generations/tasks", "video/mp4"
		f.catalog = `{"success":true,"data":{"items":[{"model_name":"doubao-seedance-2-0-fast-260128","model_type":"video","invocation_status":"available","price":{"status":"usage_dependent","version":"price-1","usage_schema":{"tokens":{"type":"number","unit":"token"},"resolution":{"enum":["480p","720p"]},"video_input":{"enum":["none","video"]}}},"media_generation_profile":{"mode":"native-doubao-video","default_variant":"generate","variants":[{"id":"generate","protocol":"doubao-video","operation":"generate","method":"POST","path":"/doubao/api/v3/contents/generations/tasks","defaults":{"resolution":"720p","ratio":"adaptive","generate_audio":true,"watermark":false},"specifications_schema":{"type":"object","additionalProperties":false,"properties":{"duration":{"type":"integer","enum":[-1,4,5,6]},"resolution":{"type":"string","enum":["480p","720p"]},"ratio":{"type":"string","enum":["16:9","adaptive"]},"generate_audio":{"type":"boolean"},"watermark":{"type":"boolean"}}},"parameter_bindings":{"duration":"duration","resolution":"resolution","ratio":"ratio","generate_audio":"generate_audio","watermark":"watermark"}}]}}]}}`
	}
	if fixedResolution {
		f.catalog = strings.Replace(f.catalog, `"type":"string","enum":["480p","720p"]`, `"type":"string","const":"720p"`, 1)
	}
	if withReference {
		studioLibraryResource(t, f, "retained-input")
		f.catalog = strings.Replace(f.catalog, `"parameter_bindings":`, `"native_reference_inputs":[{"field":"images","transport":"image-reference-array","roles":["reference_image"],"max_items":1}],"parameter_bindings":`, 1)
	}
	var previewBody []byte
	submits, previews, reads, binds := 0, 0, 0, 0
	recoveryStates := []string{"UNKNOWN", "NOT_START", "SUBMITTED", "QUEUED", "IN_PROGRESS", "SUCCESS"}
	var binding *platformclient.Binding
	asset := platformclient.Asset{AssetID: "asset_output", Owner: platformclient.MaterialOwner{Kind: "account", ID: "101"}, Kind: kind, State: "ready", Version: platformclient.AssetVersion{AssetID: "asset_output", VersionID: "version_original", FileID: "file_original"}, File: platformclient.AssetFile{FileID: "file_original", SHA256: strings.Repeat("b", 64), ByteLength: "12", ContentType: contentType, State: "ready"}}
	asset.File.Width, asset.File.Height = 720, 1280
	if kind == "video" {
		asset.File.DurationMs = 4074
	}

	f.execution = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		respond := func(v any) { _ = json.NewEncoder(w).Encode(v) }
		switch {
		case strings.HasSuffix(r.URL.Path, "/consumption-preview"):
			previews++
			var in platformclient.PreviewRequest
			_ = json.NewDecoder(r.Body).Decode(&in)
			if in.Request.ExpectedPriceVersion != "price-1" || len(in.Request.NativeRequest) == 0 {
				t.Error("preview lost native authority")
			}
			previewBody = append([]byte(nil), in.Request.NativeRequest...)
			if kind == "video" {
				var native map[string]any
				_ = json.Unmarshal(previewBody, &native)
				if native["duration"] != float64(4) || native["resolution"] != "720p" || native["ratio"] != "16:9" || native["generate_audio"] != false || native["watermark"] != false || native["tokens"] != nil || native["video_input"] != nil {
					t.Fatalf("native video parameters/usage boundary changed: %s", previewBody)
				}
			}
			respond(map[string]any{"success": true, "data": platformclient.ConsumptionPreview{CapabilityRevision: strings.Repeat("a", 64), ConnectionRevision: 7, Model: name, ModelType: kind, PriceVersion: "price-1", WalletPoints: "1.23456789", WalletMaxPoints: "1.23456789", Allowed: true, CombinationSupported: true}})
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
			case r.URL.Path == submitPath:
				submits++
				body, _ := io.ReadAll(r.Body)
				if string(body) != string(previewBody) {
					t.Error("native quote/submit bytes changed")
				}
				raw, _ := base64.RawURLEncoding.DecodeString(r.Header.Get("X-Platform-Media-Consumption"))
				var cap platformclient.MediaConsumptionConstraints
				_ = json.Unmarshal(raw, &cap)
				if cap.WalletMaxPoints != "1.23456789" || cap.ExpectedPriceVersion != "price-1" || cap.ExpectedCapabilityRevision != strings.Repeat("a", 64) || cap.ExpectedConnectionRevision != 7 {
					t.Error("exact quote/ceiling/capability/connection lost")
				}
				w.WriteHeader(503)
			case strings.HasPrefix(r.URL.Path, "/internal/relay/media-requests/"):
				reads++
				status := recoveryStates[min(reads-1, len(recoveryStates)-1)]
				respond(map[string]any{"success": true, "data": platformclient.MediaRecovery{Version: "oceanway.media-request.v1", RequestID: strings.TrimPrefix(r.URL.Path, "/internal/relay/media-requests/"), TaskID: "platform_original", Status: status, TaskAvailable: true}})
			case strings.HasSuffix(r.URL.Path, "/tasks/platform_original/outputs"):
				if reads < len(recoveryStates) {
					t.Error("read outputs before the original task succeeded")
				}
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
	if kind == "video" {
		body = `{"commandId":"video-intent","type":"canvas_video","operation":"text_to_video","prompt":"cat","model":"doubao-seedance-2-0-fast-260128","platform":{"expectedPriceVersion":"price-1","walletMaxPoints":"1.23456789"},"input":{"mode":"video","config":{"channelId":"oceanway-platform","model":"doubao-seedance-2-0-fast-260128","videoSeconds":"4","vquality":"720p","size":"16:9","videoGenerateAudio":"false","videoWatermark":"false"}}}`
	}
	body = strings.Replace(body, `"walletMaxPoints":`, `"expectedCapabilityRevision":"`+strings.Repeat("a", 64)+`","expectedConnectionRevision":7,"walletMaxPoints":`, 1)
	if withReference {
		body = strings.Replace(body, `"input":{`, `"input":{"referenceImages":[{"storageKey":"resource:retained-input","url":"https://cdn.test/not-persisted?signature=private"}],`, 1)
	}
	f.request(t, "POST", "/api/tasks/preview", "platform_session=alice", body, 200)
	w := f.request(t, "POST", "/api/tasks", "platform_session=alice", body, 200)
	var created struct {
		Data model.Task `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	for i := 0; i < 8; i++ {
		if err := f.svc.ProcessNextTask(); err != nil {
			t.Fatal(err)
		}
		f.db.Model(&model.Task{}).Where("id = ?", created.Data.ID).Update("next_poll_at", time.Now().Add(-time.Second))
		if i == 1 {
			f.restart()
		}
	}
	w = f.request(t, "GET", "/api/tasks/"+created.Data.ID, "platform_session=alice", "", 200)
	if !strings.Contains(w.Body.String(), "succeeded") || !strings.Contains(w.Body.String(), "resource:") {
		t.Fatalf("output not adopted: %s", w.Body.String())
	}
	var generated model.Resource
	if err := f.db.First(&generated, "id = ?", binding.TargetID).Error; err != nil {
		t.Fatal(err)
	}
	if generated.Width != asset.File.Width || generated.Height != asset.File.Height || generated.DurationMs != asset.File.DurationMs {
		t.Fatalf("generated resource lost the fixed original metadata: %dx%d / %dms", generated.Width, generated.Height, generated.DurationMs)
	}

	if withReference {
		var task model.Task
		if err := f.db.First(&task, "id = ?", created.Data.ID).Error; err != nil {
			t.Fatal(err)
		}
		snapshot, err := repository.New(f.db).ResourceReferenceSnapshot(task.UserID, "", []string{"retained-input"})
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := studioSnapshotReferenceIDs(snapshot, []string{"retained-input"})["retained-input"]; !ok {
			t.Errorf("completed Task lost its immutable reference input: %s", task.InputJSON)
		}
		if strings.Contains(task.InputJSON, "signature=") || strings.Contains(task.InputJSON, "material://") {
			t.Fatal("public Task leaked native request")
		}
		if !strings.Contains(string(previewBody), "material://asset_1/version_1") {
			t.Fatal("native request lost the fixed version")
		}
	}
	if submits != 1 || previews != 2 || reads != len(recoveryStates) || binds != 1 {
		t.Fatalf("duplicate lifecycle: submit=%d preview=%d read=%d binds=%d", submits, previews, reads, binds)
	}
	invalid := strings.Replace(body, kind+"-intent", "invalid-size", 1)
	if kind == "image" {
		invalid = strings.Replace(invalid, "square", "unsupported", 1)
	} else {
		invalid = strings.Replace(invalid, "720p", "720", 1)
	}
	f.request(t, "POST", "/api/tasks", "platform_session=alice", invalid, 400)
	f.request(t, "POST", "/api/tasks", "platform_session=alice", strings.Replace(strings.Replace(body, kind+"-intent", "stale-price", 1), "price-1", "old-price", 1), 409)
	// Neither invalid parameters nor a new catalog version may re-dispatch the
	// original request, whose fixed Resource remains readable.
	f.catalog = strings.ReplaceAll(f.catalog, "price-1", "price-2")
	f.request(t, "POST", "/api/tasks/preview", "platform_session=alice", body, 409)
	f.request(t, "GET", "/api/tasks/"+created.Data.ID, "platform_session=alice", "", 200)
	if submits != 1 || previews != 2 {
		t.Fatal("invalid/stale request reached the upstream")
	}
}
