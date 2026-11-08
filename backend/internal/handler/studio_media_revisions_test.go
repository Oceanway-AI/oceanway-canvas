package handler

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"yingce/backend/internal/model"
)

const studioRevisionCatalog = `{"success":true,"data":{"items":[{"model_name":"image-model","model_type":"image","invocation_status":"available","price":{"status":"available","version":"price-1"},"media_generation_profile":{"default_variant":"generate","variants":[{"id":"generate","protocol":"openai-images","operation":"generate","method":"POST","path":"/images/generations","defaults":{"n":1},"specifications_schema":{"type":"object","properties":{"n":{"type":"integer","enum":[1]}}},"parameter_bindings":{"n":"n"}}]}}]}}`

func studioRevisionBody() map[string]any {
	return map[string]any{"commandId": "revision-intent", "type": "canvas_image", "operation": "image", "prompt": "cat", "model": "image-model", "platform": map[string]any{"expectedPriceVersion": "price-1", "walletMaxPoints": "0.123456789", "expectedCapabilityRevision": strings.Repeat("a", 64), "expectedConnectionRevision": 7}, "input": map[string]any{"config": map[string]any{"channelId": "oceanway-platform", "model": "image-model"}}}
}

func TestStudioMediaRevisionsMissingOrChangedRejectBeforeAuthorization(t *testing.T) {
	for _, scenario := range []string{"request_capability", "request_connection", "preview_capability", "preview_connection", "preview_malformed", "changed_capability", "changed_connection"} {
		t.Run(scenario, func(t *testing.T) {
			f := newStudioFixture(t)
			f.catalog = studioRevisionCatalog
			body := studioRevisionBody()
			options := body["platform"].(map[string]any)
			quote := platformclient.ConsumptionPreview{CapabilityRevision: strings.Repeat("a", 64), ConnectionRevision: 7, Model: "image-model", ModelType: "image", PriceVersion: "price-1", WalletPoints: "0.123456789", Allowed: true, CombinationSupported: true}
			want, path := 409, "/api/tasks"
			switch scenario {
			case "request_capability":
				delete(options, "expectedCapabilityRevision")
				want = 400
			case "request_connection":
				delete(options, "expectedConnectionRevision")
				want = 400
			case "preview_capability":
				quote.CapabilityRevision = ""
				path += "/preview"
			case "preview_connection":
				quote.ConnectionRevision = 0
				path += "/preview"
			case "preview_malformed":
				quote.CapabilityRevision = "not-a-published-revision"
				path += "/preview"
			case "changed_capability":
				quote.CapabilityRevision = strings.Repeat("b", 64)
			case "changed_connection":
				quote.ConnectionRevision = 8
			}
			f.execution = func(w http.ResponseWriter, r *http.Request) {
				if !strings.HasSuffix(r.URL.Path, "/consumption-preview") {
					t.Error("rejected quote reached execution: " + r.URL.Path)
					w.WriteHeader(500)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": quote})
			}
			raw, _ := json.Marshal(body)
			f.request(t, "POST", path, "platform_session=alice", string(raw), want)
			for _, table := range []any{&model.Task{}, &model.StudioExecution{}, &model.BillingOrder{}} {
				var count int64
				if err := f.db.Model(table).Count(&count).Error; err != nil || count != 0 {
					t.Fatalf("rejected quote persisted %T: count=%d err=%v", table, count, err)
				}
			}
		})
	}
}

func TestStudioMediaAuthorizationRecoveryKeepsOriginalRevisions(t *testing.T) {
	for _, changed := range []string{"capability", "connection"} {
		t.Run(changed, func(t *testing.T) {
			f := newStudioFixture(t)
			f.catalog = studioRevisionCatalog
			quotes, issues, submits, recovers := 0, 0, 0, 0
			taskID, requestID := "", ""
			capability, connection := strings.Repeat("a", 64), int64(7)
			f.execution = func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				respond := func(v any) { _ = json.NewEncoder(w).Encode(v) }
				switch {
				case strings.HasSuffix(r.URL.Path, "/consumption-preview"):
					quotes++
					respond(map[string]any{"success": true, "data": platformclient.ConsumptionPreview{CapabilityRevision: capability, ConnectionRevision: connection, Model: "image-model", ModelType: "image", PriceVersion: "price-1", WalletPoints: "0.123456789", Allowed: true, CombinationSupported: true}})
				case r.URL.Path == "/api/platform/v1/execution-authorizations":
					issues++
					var in map[string]string
					_ = json.NewDecoder(r.Body).Decode(&in)
					if taskID != "" && taskID != in["task_reference"] {
						t.Error("authorization recovery changed task identity")
					}
					taskID = in["task_reference"]
					if issues == 1 {
						w.WriteHeader(503)
						return
					}
					respond(map[string]any{"authorization_id": "exa_original", "product": "studio", "task_reference": taskID, "expires_at": time.Now().Add(time.Hour).Unix()})
				case r.URL.Path == "/internal/relay/images/generations":
					submits++
					requestID = r.Header.Get("X-Platform-Media-Request-Id")
					raw, _ := base64.RawURLEncoding.DecodeString(r.Header.Get("X-Platform-Media-Consumption"))
					var cap platformclient.MediaConsumptionConstraints
					if json.Unmarshal(raw, &cap) != nil || cap.ExpectedCapabilityRevision != strings.Repeat("a", 64) || cap.ExpectedConnectionRevision != 7 || cap.ExpectedPriceVersion != "price-1" || cap.WalletMaxPoints != "0.123456789" {
						t.Errorf("recovery lost original frozen consumption: %s", raw)
					}
					if requestID == "" || r.Header.Get("X-Platform-Execution-Authorization") != "exa_original" {
						t.Error("original request authority changed")
					}
					// Platform rejects drift before its supplier invocation.
					if cap.ExpectedCapabilityRevision == capability && cap.ExpectedConnectionRevision == connection {
						t.Error("stale revisions were silently replaced")
					}
					w.WriteHeader(409)
				case strings.HasPrefix(r.URL.Path, "/internal/relay/media-requests/"):
					recovers++
					if !strings.HasSuffix(r.URL.Path, "/"+requestID) {
						t.Error("recovered a new request")
					}
					respond(map[string]any{"success": true, "data": platformclient.MediaRecovery{RequestID: requestID, Status: "FAILURE", NoExecution: true}})
				case strings.HasSuffix(r.URL.Path, "/release"):
					w.WriteHeader(204)
				default:
					t.Error("unexpected request: " + r.URL.Path)
					w.WriteHeader(500)
				}
			}
			raw, _ := json.Marshal(studioRevisionBody())
			f.request(t, "POST", "/api/tasks", "platform_session=alice", string(raw), 503)
			if changed == "capability" {
				capability = strings.Repeat("b", 64)
			} else {
				connection = 8
			}
			f.request(t, "POST", "/api/tasks", "platform_session=alice", string(raw), 200)
			for i := 0; i < 3; i++ {
				f.db.Model(&model.Task{}).Where("id = ?", taskID).Update("next_poll_at", time.Now().Add(-time.Second))
				if err := f.svc.ProcessNextTask(); err != nil {
					t.Fatal(err)
				}
			}
			w := f.request(t, "GET", "/api/tasks/"+taskID, "platform_session=alice", "", 200)
			if !strings.Contains(w.Body.String(), `"status":"failed"`) {
				t.Fatalf("revision rejection did not settle original task: %s", w.Body.String())
			}
			if quotes != 1 || issues != 2 || submits != 1 || recovers != 1 {
				t.Fatalf("new preview/authority/request on recovery: %d/%d/%d/%d", quotes, issues, submits, recovers)
			}
		})
	}
}
