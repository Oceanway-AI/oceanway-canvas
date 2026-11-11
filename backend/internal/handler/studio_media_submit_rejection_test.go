package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"yingce/backend/internal/model"
)

func TestStudioMediaInitialRejectionRequiresOriginalNoExecutionReceipt(t *testing.T) {
	for _, mode := range []string{"definite", "http400", "other_request", "wrong_version", "stored_rejection", "http503"} {
		t.Run(mode, func(t *testing.T) {
			f := newStudioFixture(t)
			f.catalog = studioRevisionCatalog
			submits, recovers := 0, 0
			requestID := ""
			f.execution = func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				respond := func(v any) { _ = json.NewEncoder(w).Encode(v) }
				switch {
				case strings.HasSuffix(r.URL.Path, "/consumption-preview"):
					respond(map[string]any{"success": true, "data": platformclient.ConsumptionPreview{Model: "image-model", ModelType: "image", PriceVersion: "price-1", CapabilityRevision: strings.Repeat("a", 64), ConnectionRevision: 7, WalletPoints: "0.123456789", Allowed: true, CombinationSupported: true}})
				case r.URL.Path == "/api/platform/v1/execution-authorizations":
					var b struct {
						TaskReference string `json:"task_reference"`
					}
					_ = json.NewDecoder(r.Body).Decode(&b)
					respond(platformclient.ExecutionAuthorization{AuthorizationID: "exa1.fixture", Product: "studio", TaskReference: b.TaskReference, ExpiresAt: time.Now().Add(time.Hour).Unix()})
				case strings.HasSuffix(r.URL.Path, "/release"):
					respond(map[string]any{"success": true, "data": map[string]any{}})
				case r.URL.Path == "/internal/relay/images/generations":
					submits++
					requestID = r.Header.Get("X-Platform-Media-Request-Id")
					receipt := platformclient.MediaRecovery{Version: "oceanway.media-request.v1", RequestID: requestID, Status: "FAILURE", NoExecution: true}
					switch mode {
					case "http400", "http503":
						receipt.NoExecution = false
					case "other_request":
						receipt.RequestID = "other"
					case "wrong_version":
						receipt.Version = "other"
					case "stored_rejection":
						receipt.TaskID = "local-rejection-task"
						receipt.TaskAvailable = true
					}
					status := 400
					if mode == "http503" {
						status = 503
					}
					w.WriteHeader(status)
					respond(map[string]any{"success": false, "data": receipt, "message": "private secret https://supplier/key"})
				case r.URL.Path == "/internal/relay/media-requests/"+requestID:
					recovers++
					w.WriteHeader(202)
					respond(map[string]any{"success": true, "data": platformclient.MediaRecovery{Version: "oceanway.media-request.v1", RequestID: requestID, Status: "pending"}})
				default:
					t.Error("unexpected request " + r.URL.Path)
					w.WriteHeader(500)
				}
			}
			raw, _ := json.Marshal(studioRevisionBody())
			w := f.request(t, "POST", "/api/tasks", "platform_session=alice", string(raw), 200)
			var created struct {
				Data model.Task `json:"data"`
			}
			_ = json.Unmarshal(w.Body.Bytes(), &created)
			for i := 0; i < 2; i++ {
				if e := f.svc.ProcessNextTask(); e != nil {
					t.Fatal(e)
				}
				f.db.Model(&model.Task{}).Where("id = ?", created.Data.ID).Update("next_poll_at", time.Now().Add(-time.Second))
			}
			var task model.Task
			var execution model.StudioExecution
			if e := f.db.First(&task, "id = ?", created.Data.ID).Error; e != nil {
				t.Fatal(e)
			}
			if e := f.db.First(&execution, "task_id = ?", created.Data.ID).Error; e != nil {
				t.Fatal(e)
			}
			if submits != 1 {
				t.Fatalf("resubmitted %d", submits)
			}
			if strings.Contains(task.ExecutionDiagnosticJSON, "private") || strings.Contains(task.ExecutionDiagnosticJSON, "supplier") {
				t.Fatal("leaked upstream diagnostic")
			}
			if mode == "definite" || mode == "stored_rejection" {
				if task.Status != model.TaskStatusFailed || execution.State != "completed" || execution.ResponseEncrypted == "" || recovers != 0 || !strings.Contains(task.ExecutionDiagnosticJSON, "rejected_without_execution") {
					t.Fatalf("explicit reject not terminal: %s %s recovery=%d", task.Status, execution.State, recovers)
				}
			} else {
				if task.Status != model.TaskStatusRunning || execution.State != "submitted" || recovers != 1 || !strings.Contains(task.ExecutionDiagnosticJSON, "recover_original") {
					t.Fatalf("ambiguous outcome not recovered: %s %s recovery=%d", task.Status, execution.State, recovers)
				}
			}
		})
	}
}

func TestStudioMediaCachedUnknownRecoversOriginalFailure(t *testing.T) {
	for _, mode := range []string{"no_execution", "failed", "failed_discarded", "other_task", "other_request", "wrong_version"} {
		t.Run(mode, func(t *testing.T) {
			f := newStudioFixture(t)
			f.catalog = studioRevisionCatalog
			submits, recovers, outputs := 0, 0, 0
			requestID := ""
			f.execution = func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				respond := func(v any) { _ = json.NewEncoder(w).Encode(v) }
				switch {
				case strings.HasSuffix(r.URL.Path, "/consumption-preview"):
					respond(map[string]any{"success": true, "data": platformclient.ConsumptionPreview{Model: "image-model", ModelType: "image", PriceVersion: "price-1", CapabilityRevision: strings.Repeat("a", 64), ConnectionRevision: 7, WalletPoints: "0.123456789", Allowed: true, CombinationSupported: true}})
				case r.URL.Path == "/api/platform/v1/execution-authorizations":
					var b struct {
						TaskReference string `json:"task_reference"`
					}
					_ = json.NewDecoder(r.Body).Decode(&b)
					respond(platformclient.ExecutionAuthorization{AuthorizationID: "exa1.fixture", Product: "studio", TaskReference: b.TaskReference, ExpiresAt: time.Now().Add(time.Hour).Unix()})
				case strings.HasSuffix(r.URL.Path, "/release"):
					respond(map[string]any{"success": true})
				case r.URL.Path == "/internal/relay/images/generations":
					submits++
					requestID = r.Header.Get("X-Platform-Media-Request-Id")
					w.WriteHeader(503)
				case r.URL.Path == "/internal/relay/media-requests/"+requestID:
					recovers++
					receipt := platformclient.MediaRecovery{Version: "oceanway.media-request.v1", RequestID: requestID, TaskID: "original-task", TaskAvailable: true, Status: "UNKNOWN"}
					if recovers > 1 {
						receipt.Status = "FAILURE"
						switch mode {
						case "no_execution":
							receipt.NoExecution = true
						case "failed_discarded":
							receipt.TaskAvailable = false
						case "other_task":
							receipt.TaskID = "different-task"
						case "other_request":
							receipt.RequestID = "different-request"
						case "wrong_version":
							receipt.Version = "other"
						}
					}
					respond(map[string]any{"success": true, "data": receipt})
				case strings.HasSuffix(r.URL.Path, "/outputs"):
					outputs++
					w.WriteHeader(404)
				default:
					t.Error("unexpected request " + r.URL.Path)
					w.WriteHeader(500)
				}
			}
			raw, _ := json.Marshal(studioRevisionBody())
			w := f.request(t, "POST", "/api/tasks", "platform_session=alice", string(raw), 200)
			var created struct {
				Data model.Task `json:"data"`
			}
			_ = json.Unmarshal(w.Body.Bytes(), &created)
			var cached string
			for i := 0; i < 3; i++ {
				if err := f.svc.ProcessNextTask(); err != nil {
					t.Fatal(err)
				}
				f.db.Model(&model.Task{}).Where("id = ?", created.Data.ID).Update("next_poll_at", time.Now().Add(-time.Second))
				if i == 1 {
					var execution model.StudioExecution
					if err := f.db.First(&execution, "task_id = ?", created.Data.ID).Error; err != nil {
						t.Fatal(err)
					}
					cached = execution.ResponseEncrypted
					if cached == "" {
						t.Fatal("original UNKNOWN identity was not frozen")
					}
					f.restart()
				}
			}
			var task model.Task
			var execution model.StudioExecution
			if err := f.db.First(&task, "id = ?", created.Data.ID).Error; err != nil {
				t.Fatal(err)
			}
			if err := f.db.First(&execution, "task_id = ?", created.Data.ID).Error; err != nil {
				t.Fatal(err)
			}
			if submits != 1 || recovers != 2 || outputs != 0 {
				t.Fatalf("original recovery changed: submit=%d recover=%d outputs=%d", submits, recovers, outputs)
			}
			if mode == "no_execution" || mode == "failed" || mode == "failed_discarded" {
				if task.Status != model.TaskStatusFailed || execution.State != "completed" || execution.ResponseEncrypted == cached {
					t.Fatalf("original terminal not persisted: %s/%s", task.Status, execution.State)
				}
				if (mode == "no_execution") != strings.Contains(task.Error, "未执行生成") || strings.Contains(task.Error, "退款") {
					t.Fatalf("execution/financial fact invented: %s", task.Error)
				}
			} else if task.Status != model.TaskStatusRunning || execution.State != "submitted" || execution.ResponseEncrypted != cached {
				t.Fatal("unrelated receipt replaced the frozen original task")
			}
		})
	}
}
