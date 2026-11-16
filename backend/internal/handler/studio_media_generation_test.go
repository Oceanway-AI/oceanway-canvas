package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"github.com/gin-gonic/gin"
	"yingce/backend/internal/model"
	"yingce/backend/internal/repository"
)

const studioGenerationCatalog = `{"success":true,"data":{"items":[{"model_name":"exact-image","model_type":"image","invocation_status":"available","price":{"status":"available","version":"price-1"},"media_generation_profile":{"default_variant":"generate","variants":[{"id":"generate","media_generation_v1":true,"protocol":"openai-images","operation":"generate","method":"POST","path":"/images/generations","defaults":{"n":1,"size":"square"},"specifications_schema":{"type":"object","properties":{"n":{"type":"integer","minimum":1,"maximum":3},"size":{"enum":["square"]}},"required":["n","size"]},"parameter_bindings":{"n":"n","size":"size"}}]}}]}}`
const studioGenerationInput = `{"commandId":"original-v1","type":"canvas_image","operation":"image","prompt":"cat","model":"exact-image","platform":{"expectedPriceVersion":"price-1","walletMaxPoints":"0"},"input":{"mode":"image","config":{"channelId":"oceanway-platform","model":"exact-image","imageParameters":{"count":1,"size":"square"}}}}`

func TestStudioMediaGenerationOriginalAdmissionAndRead(t *testing.T) {
	for _, lost := range []bool{false, true} {
		t.Run(map[bool]string{false: "received", true: "lost"}[lost], func(t *testing.T) {
			f := newStudioFixture(t)
			f.catalog = studioGenerationCatalog
			var requestID, productTask string
			posts, reads := 0, 0
			f.execution = func(w http.ResponseWriter, r *http.Request) {
				if !strings.HasPrefix(r.URL.Path, "/internal/products/v1/media-generations") {
					t.Errorf("unexpected execution API %s", r.URL.Path)
					w.WriteHeader(404)
					return
				}
				if r.Header.Get("Cookie") == "" || r.Header.Get("X-Platform-Product-Key") == "" {
					t.Error("missing current product/session")
				}
				w.Header().Set("Content-Type", "application/json")
				switch r.Method {
				case "POST":
					posts++
					var in platformclient.MediaCommand
					if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
						t.Error(err)
					}
					requestID, productTask = in.RequestID, in.ProductTaskID
					if requestID != "original-v1" || productTask == "" || in.VariantID != "generate" || in.Model != "exact-image" || in.Consumption == nil || in.Consumption.WalletMaxPoints != "0" || !strings.Contains(string(in.NativeRequest), `"prompt":"cat"`) {
						t.Errorf("bad frozen command")
					}
					if lost {
						w.WriteHeader(503)
						return
					}
				case "GET":
					reads++
					if r.URL.Path != "/internal/products/v1/media-generations/"+requestID {
						t.Error("read changed request id")
					}
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"request_id": requestID, "task_id": "platform-task-1", "revision": 1 + reads, "execution_state": "running", "funds_state": "reserved", "outputs": []any{}, "retry_after_ms": 2500})
			}
			auth := "platform_session=alice"
			f.request(t, "GET", "/api/model-catalog", auth, "", 200)
			w := f.request(t, "POST", "/api/tasks", auth, studioGenerationInput, 200)
			var wire struct{ Data model.Task }
			_ = json.Unmarshal(w.Body.Bytes(), &wire)
			if posts != 1 || reads != 0 || f.catalogCalls != 1 || wire.Data.ID != productTask || wire.Data.Status != model.TaskStatusRunning {
				t.Fatalf("submission posts=%d reads=%d catalogs=%d task=%+v", posts, reads, f.catalogCalls, wire.Data)
			}
			if claimed, e := repository.New(f.db).ClaimNextTask("ordinary-worker", time.Minute); e != nil || claimed != nil {
				t.Fatalf("direct task in local worker queue: %v %v", claimed, e)
			}
			f.request(t, "POST", "/api/tasks", auth, studioGenerationInput, 200)
			if posts != 1 {
				t.Fatal("duplicate dispatch")
			}
			f.restart()
			f.router = gin.New()
			if err := RegisterStudioAPI(f.router.Group("/api"), f.svc); err != nil {
				t.Fatal(err)
			}
			f.request(t, "GET", "/api/tasks/by-command/original-v1", auth, "", 200)
			if reads != 1 {
				t.Fatalf("read count %d", reads)
			}
			f.request(t, "GET", "/api/tasks/"+productTask, "platform_session=bob", "", 404)
			if reads != 1 {
				t.Fatal("another user queried original")
			}
			f.request(t, "POST", "/api/tasks", auth, strings.Replace(strings.Replace(studioGenerationInput, "original-v1", "wrong-space", 1), `"expectedPriceVersion"`, `"workspaceId":"team_foreign","expectedPriceVersion"`, 1), 403)
			f.request(t, "POST", "/api/tasks", auth, strings.Replace(studioGenerationInput, "cat", "dog", 1), 409)
			if posts != 1 {
				t.Fatal("conflict dispatched")
			}
		})
	}
}

func TestStudioMediaGenerationSlotsAndEarlyBrowser(t *testing.T) {
	studioMediaGenerationSlots(t, false, false, false)
}
func TestStudioMediaGenerationSynchronous(t *testing.T) {
	studioMediaGenerationSlots(t, true, false, false)
}
func TestStudioMediaGenerationOriginalBindingLostAck(t *testing.T) {
	studioMediaGenerationSlots(t, true, true, false)
}
func TestStudioMediaGenerationBrowserAdoptsFromOriginalReceipt(t *testing.T) {
	studioMediaGenerationSlots(t, false, false, true)
}
func studioMediaGenerationSlots(t *testing.T, syncReady, loseBinding, browserRead bool) {
	f := newStudioFixture(t)
	f.catalog = studioGenerationCatalog
	var reqID string
	reads, posts, tickets := 0, 0, 0
	bindings := map[string]platformclient.Binding{}
	f.materials = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/bindings"):
			if b, ok := bindings[r.URL.Query().Get("command_id")]; ok {
				_ = json.NewEncoder(w).Encode(b)
			} else {
				w.WriteHeader(404)
			}
		case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/bindings"):
			var in platformclient.BindingRequest
			_ = json.NewDecoder(r.Body).Decode(&in)
			if _, ok := bindings[in.CommandID]; ok {
				t.Error("duplicate adopt")
			}
			parts := strings.Split(r.URL.Path, "/")
			asset := parts[len(parts)-2]
			b := platformclient.Binding{BindingID: "binding_" + asset, AssetID: asset, VersionID: in.VersionID, TargetApp: "studio", TargetType: in.TargetType, TargetID: in.TargetID, Slot: in.Slot, State: "active"}
			bindings[in.CommandID] = b
			if loseBinding && asset == "asset_one" {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			_ = json.NewEncoder(w).Encode(b)
		default:
			t.Errorf("unexpected material request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(500)
		}
	}
	f.execution = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/outputs/1/preview") {
			tickets++
			w.Header().Set("Location", "/api/platform/v1/media-previews/task-original/1?ticket=fixture")
			w.WriteHeader(302)
			return
		}
		if r.Method == "POST" {
			posts++
			var in platformclient.MediaCommand
			_ = json.NewDecoder(r.Body).Decode(&in)
			reqID = in.RequestID
		} else {
			reads++
		}
		out := []map[string]any{}
		for i, name := range []string{"zero", "one", "two"} {
			o := map[string]any{"index": i, "kind": "image", "content_state": "available", "preview_state": "available", "material_state": "saved", "asset_id": "asset_" + name, "version_id": "version_" + name, "file_id": "file_" + name, "sha256": strings.Repeat("b", 64), "bytes": 12, "mime_type": "image/png", "owner": map[string]string{"kind": "account", "id": "101"}, "width": 24, "height": 13}
			if !syncReady && i == 1 && reads == 0 {
				o = map[string]any{"index": 1, "kind": "image", "content_state": "available", "preview_state": "available", "material_state": "failed", "error": map[string]any{"code": "material_archive_unavailable", "category": "platform", "stage": "archive", "recovery": map[string]string{"action": "query_original", "availability": "available"}}, "preview_path": "/internal/products/v1/media-generations/" + reqID + "/outputs/1/preview"}
			}
			out = append(out, o)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"request_id": reqID, "task_id": "task-original", "revision": reads + 1, "execution_state": "succeeded", "funds_state": "settled", "outputs": out, "retry_after_ms": 1200})
	}
	auth := "platform_session=alice"
	result := f.request(t, "POST", "/api/tasks", auth, strings.Replace(studioGenerationInput, `"count":1`, `"count":3`, 1), 200)
	var w struct{ Data model.Task }
	_ = json.Unmarshal(result.Body.Bytes(), &w)
	var projection struct {
		Outputs []struct {
			Index                  int
			ResourceID, PreviewURL string
		}
	}
	_ = json.Unmarshal([]byte(w.Data.ResultJSON), &projection)
	if browserRead {
		path := "/api/tasks/" + w.Data.ID
		bodyless := func(method, path string, want int) *httptest.ResponseRecorder {
			req := httptest.NewRequest(method, "https://studio.example.test"+path, nil)
			req.Header.Set("Cookie", auth)
			reply := httptest.NewRecorder()
			f.router.ServeHTTP(reply, req)
			if reply.Code != want {
				t.Fatalf("%s %s status=%d want=%d", method, path, reply.Code, want)
			}
			return reply
		}
		raw := bodyless("GET", path, 200)
		var pending struct{ Data model.Task }
		_ = json.Unmarshal(raw.Body.Bytes(), &pending)
		if !pending.Data.PlatformProjectionPending || len(bindings) != 2 || reads != 1 {
			t.Fatalf("same-origin browser GET lost unbound original, bindings=%d reads=%d", len(bindings), reads)
		}
		bodyless("POST", path+"/adopt-outputs", 403)
		f.request(t, "POST", path+"/adopt-outputs", "platform_session=bob", "", 404)
		for i := 0; i < 2; i++ {
			saved := f.request(t, "POST", path+"/adopt-outputs", auth, "", 200)
			var done struct{ Data model.Task }
			_ = json.Unmarshal(saved.Body.Bytes(), &done)
			if done.Data.Status != model.TaskStatusSucceeded || done.Data.PlatformProjectionPending || len(bindings) != 3 || posts != 1 || reads != 1 {
				t.Fatalf("adoption replay status=%s bindings=%d post=%d get=%d", done.Data.Status, len(bindings), posts, reads)
			}
		}
		return
	}
	if loseBinding {
		if w.Data.Status != model.TaskStatusRunning || len(bindings) != 3 || projection.Outputs[0].ResourceID == "" || projection.Outputs[1].ResourceID != "" || projection.Outputs[2].ResourceID == "" {
			t.Fatalf("wrong initial local binding projection %+v", projection)
		}
		f.restart()
		saved := f.request(t, "GET", "/api/tasks/"+w.Data.ID, auth, "", 200)
		_ = json.Unmarshal(saved.Body.Bytes(), &w)
		if w.Data.Status != model.TaskStatusSucceeded || reads != 0 || posts != 1 || len(bindings) != 3 {
			t.Fatalf("local original binding recovery state=%s reads=%d posts=%d bindings=%d", w.Data.Status, reads, posts, len(bindings))
		}
		return
	}
	if syncReady {
		if w.Data.Status != model.TaskStatusSucceeded || len(bindings) != 3 || posts != 1 || reads != 0 {
			t.Fatalf("synchronous state %s, binds=%d, POST=%d GET=%d", w.Data.Status, len(bindings), posts, reads)
		}
		f.request(t, "GET", "/api/tasks/"+w.Data.ID, auth, "", 200)
		if reads != 0 {
			t.Fatal("synchronous receipt required another Platform status GET")
		}
		return
	}
	if len(projection.Outputs) != 3 || projection.Outputs[0].ResourceID == "" || projection.Outputs[2].ResourceID == "" || projection.Outputs[1].ResourceID != "" || projection.Outputs[1].PreviewURL == "" || len(bindings) != 2 || posts != 1 || reads != 0 {
		t.Fatalf("initial partial outputs %+v bindings=%d reads=%d", projection.Outputs, len(bindings), reads)
	}
	early := f.request(t, "GET", projection.Outputs[1].PreviewURL, auth, "", 307)
	if early.Header().Get("Location") == "" || early.Header().Get("Cache-Control") != "private, no-store" || tickets != 1 || reads != 0 {
		t.Fatalf("preview tickets=%d reads=%d headers=%v", tickets, reads, early.Header())
	}
	f.request(t, "GET", projection.Outputs[1].PreviewURL, "platform_session=bob", "", 404)
	if tickets != 1 {
		t.Fatal("foreign preview fetched")
	}
	saved := f.request(t, "GET", "/api/tasks/"+w.Data.ID, auth, "", 200)
	_ = json.Unmarshal(saved.Body.Bytes(), &w)
	if w.Data.Status != model.TaskStatusSucceeded || len(bindings) != 3 || reads != 1 || posts != 1 {
		t.Fatalf("final %s binds=%d reads=%d posts=%d", w.Data.Status, len(bindings), reads, posts)
	}
	f.request(t, "GET", "/api/tasks/"+w.Data.ID, auth, "", 200)
	if reads != 1 || len(bindings) != 3 {
		t.Fatal("terminal already-complete task requested another status or adoption")
	}
}

func TestStudioMediaGenerationConfirmsOnlyMatchingRejection(t *testing.T) {
	for _, match := range []bool{true, false} {
		t.Run(map[bool]string{true: "matching", false: "different_request"}[match], func(t *testing.T) {
			f := newStudioFixture(t)
			f.catalog = studioGenerationCatalog
			n := 0
			f.execution = func(w http.ResponseWriter, r *http.Request) {
				n++
				var cmd platformclient.MediaCommand
				_ = json.NewDecoder(r.Body).Decode(&cmd)
				ref := cmd.RequestID
				if !match {
					ref = "different"
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusPaymentRequired)
				_ = json.NewEncoder(w).Encode(map[string]any{"request_id": ref, "error": map[string]string{"code": "insufficient_quota", "message": "Payment Required", "stage": "admission_rejected", "request_id": ref}})
			}
			w := f.request(t, "POST", "/api/tasks", "platform_session=alice", studioGenerationInput, 200)
			var result struct{ Data model.Task }
			_ = json.Unmarshal(w.Body.Bytes(), &result)
			if n != 1 || result.Data.PlatformGenerationRequest != "original-v1" || result.Data.Diagnostic == nil {
				t.Fatalf("wrong original %+v", result.Data)
			}
			if match {
				if result.Data.Status != model.TaskStatusFailed || !strings.Contains(result.Data.Error, "额度") {
					t.Fatalf("rejection not actionable %+v", result.Data)
				}
			} else if result.Data.Status != model.TaskStatusRunning {
				t.Fatal("another request rejected the original")
			}
		})
	}
}

func TestStudioMediaGenerationObservesRemainingFundsAndPermanentOutputs(t *testing.T) {
	for _, mode := range []string{"failed_reserved", "succeeded_reserved", "permanent_output"} {
		t.Run(mode, func(t *testing.T) {
			f := newStudioFixture(t)
			f.catalog = studioGenerationCatalog
			var original string
			posts, reads := 0, 0
			f.execution = func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" {
					posts++
					var cmd platformclient.MediaCommand
					_ = json.NewDecoder(r.Body).Decode(&cmd)
					original = cmd.RequestID
				} else {
					reads++
					if !strings.HasSuffix(r.URL.Path, "/"+original) {
						t.Error("wrong original")
					}
				}
				g := map[string]any{"request_id": original, "task_id": "original-observed", "revision": reads + 1, "execution_state": "succeeded", "funds_state": "reserved", "retry_after_ms": 1500, "outputs": []any{}}
				if mode == "failed_reserved" {
					g["execution_state"] = "failed"
				}
				if mode == "permanent_output" || reads > 0 {
					g["retry_after_ms"] = 0
					g["funds_state"] = "settled"
					g["outputs"] = []map[string]any{{"index": 0, "kind": "image", "content_state": "failed", "preview_state": "failed", "material_state": "failed", "error": map[string]string{"code": "result_capture_failed", "stage": "result_capture", "category": "platform"}}}
				}
				_ = json.NewEncoder(w).Encode(g)
			}
			auth := "platform_session=alice"
			result := f.request(t, "POST", "/api/tasks", auth, studioGenerationInput, 200)
			var w struct{ Data model.Task }
			_ = json.Unmarshal(result.Body.Bytes(), &w)
			if mode != "permanent_output" {
				if w.Data.Status != model.TaskStatusRunning || w.Data.PlatformRetryAfterMS != 1500 {
					t.Fatalf("lost remaining settlement %+v", w.Data.Status)
				}
				result = f.request(t, "GET", "/api/tasks/"+w.Data.ID, auth, "", 200)
				w.Data = model.Task{}
				_ = json.Unmarshal(result.Body.Bytes(), &w)
			}
			if w.Data.Status != model.TaskStatusFailed || w.Data.PlatformRetryAfterMS != 0 {
				t.Fatalf("permanent output still observes status=%s delay=%d", w.Data.Status, w.Data.PlatformRetryAfterMS)
			}
			originalReads := reads
			f.request(t, "GET", "/api/tasks/"+w.Data.ID, auth, "", 200)
			if posts != 1 || reads != originalReads || (mode == "permanent_output" && reads != 0) {
				t.Fatalf("terminal traffic posts=%d reads=%d", posts, reads)
			}
		})
	}
}

func TestStudioMediaGenerationOriginalReadRecoversConfirmedRejection(t *testing.T) {
	f := newStudioFixture(t)
	f.catalog = studioGenerationCatalog
	var posts, reads int
	f.execution = func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			posts++
			w.WriteHeader(502)
			return
		}
		reads++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(409)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": "price_changed", "message": "price changed", "stage": "admission_rejected", "request_id": "original-v1"}})
	}
	auth := "platform_session=alice"
	var task struct{ Data model.Task }
	first := f.request(t, "POST", "/api/tasks", auth, studioGenerationInput, 200)
	_ = json.Unmarshal(first.Body.Bytes(), &task)
	if task.Data.Status != model.TaskStatusRunning {
		t.Fatal("ambiguous receipt incorrectly rejected")
	}
	saved := f.request(t, "GET", "/api/tasks/"+task.Data.ID, auth, "", 200)
	_ = json.Unmarshal(saved.Body.Bytes(), &task)
	if task.Data.Status != model.TaskStatusFailed || !strings.Contains(task.Data.Error, "价格") || reads != 1 || posts != 1 {
		t.Fatalf("original read status=%s reads=%d posts=%d", task.Data.Status, reads, posts)
	}
}
