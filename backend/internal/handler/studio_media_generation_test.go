package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
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
					if requestID == "" || productTask == "" || in.VariantID != "generate" || in.Model != "exact-image" || in.Consumption == nil || in.Consumption.WalletMaxPoints != "0" || !strings.Contains(string(in.NativeRequest), `"prompt":"cat"`) {
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
			f.request(t, "GET", "/api/tasks/by-command/original-v1", auth, "", 200)
			if reads != 1 {
				t.Fatalf("read count %d", reads)
			}
			f.request(t, "GET", "/api/tasks/"+productTask, "platform_session=bob", "", 404)
			if reads != 1 {
				t.Fatal("another user queried original")
			}
			f.request(t, "POST", "/api/tasks", auth, strings.Replace(studioGenerationInput, "cat", "dog", 1), 409)
			if posts != 1 {
				t.Fatal("conflict dispatched")
			}
		})
	}
}
