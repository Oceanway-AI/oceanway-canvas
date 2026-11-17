package app

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"yingce/backend/internal/model"
	"yingce/backend/internal/platformintegration"
)

func TestStudioAgentTextRetryOnlyAfterOriginalNoExecution(t *testing.T) {
	for _, tc := range []struct {
		name, code     string
		http, succeeds int
		wantPosts      int
		cancelAt       int
		failure        *platformclient.TextFailure
	}{
		{"three_rejections_then_lost_success", "system_memory_overloaded", 503, 4, 4, 0, nil},
		{"four_rejections_stop", "", 503, 0, 4, 0, nil},
		{"unknown_only_queries_original", "", 504, 1, 1, 0, nil},
		{"rate_limit", "rate_limit_error", 429, 2, 2, 0, nil},
		{"permanent_forbidden", "PLATFORM_MODEL_UNAVAILABLE", 403, 0, 1, 0, nil},
		{"permanent_model_even_on_503", "model_not_found", 503, 0, 1, 0, nil},
		{"cancel_before_proof", "", 503, 0, 1, 1, nil},
		{"cancel_during_backoff", "", 503, 0, 1, 2, nil},
		{"explicit_parameter_rejection", "invalid_request_error", 400, 0, 1, 0, &platformclient.TextFailure{Code: "invalid_request_error", HTTPStatus: 400, NoExecution: true}},
		{"explicit_context_overflow", "context_length_exceeded", 400, 0, 1, 0, &platformclient.TextFailure{Code: "context_length_exceeded", HTTPStatus: 400, NoExecution: true}},
		{"explicit_rate_limit", "rate_limit_error", 429, 2, 2, 0, &platformclient.TextFailure{Code: "rate_limit_error", HTTPStatus: 429, NoExecution: true, RetryAllowed: true}},
		{"retry_denied", "server_error", 503, 0, 1, 0, &platformclient.TextFailure{Code: "server_error", HTTPStatus: 503, NoExecution: true}},
		{"historical_parameter_rejection", "invalid_request_error", 400, 0, 1, 1, &platformclient.TextFailure{Code: "text_request_failed"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, db, _, _ := creationTestService(t)
			if err := db.AutoMigrate(&model.StudioExecution{}, &model.StudioMaterialReference{}); err != nil {
				t.Fatal(err)
			}
			s.legacyCloudAgentRootTask = false
			s.activeCancels = make(map[string]context.CancelFunc)
			var ids []string
			var fp string
			polls := map[string]int{}
			grants, executions, charges := 0, 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.HasSuffix(r.URL.Path, "/models"):
					io.WriteString(w, `{"success":true,"data":{"items":[{"model_name":"m","model_type":"text","invocation_status":"published_text","supported_protocols":["openai-chat"],"price":{"status":"available","version":"p1"},"content":{"text_capability":{"context_window_tokens":32000,"max_output_tokens":4096,"max_images":1,"max_image_bytes":1048576}}}]}}`)
				case r.URL.Path == "/api/platform/v1/execution-authorizations":
					grants++
					var in map[string]string
					json.NewDecoder(r.Body).Decode(&in)
					json.NewEncoder(w).Encode(map[string]any{"authorization_id": "exa_retry", "product": "studio", "task_reference": in["task_reference"], "expires_at": time.Now().Add(time.Hour).Unix()})
				case strings.HasSuffix(r.URL.Path, "/release"):
					w.WriteHeader(204)
				case r.URL.Path == "/internal/relay/chat/completions" && r.Method == "POST":
					w.Header().Set("X-Oneapi-Request-Id", "platform-original-relay")
					id := r.Header.Get("X-Platform-Text-Request-Id")
					body, _ := io.ReadAll(r.Body)
					fingerprint := fmt.Sprintf("%x", sha256.Sum256(append([]byte("openai-chat\x00"), body...)))
					if id == "" || r.Header.Get("X-Platform-Execution-Authorization") != "exa_retry" || (fp != "" && fp != fingerprint) {
						t.Error("request lost its identity, original authority or frozen body")
					}
					for _, sent := range ids {
						if sent == id {
							t.Error("same request posted twice")
						}
					}
					fp = fingerprint
					ids = append(ids, id)
					if len(ids) == tc.succeeds {
						executions++
						charges++
					}
					w.WriteHeader(tc.http)
					json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": tc.code}})
				case strings.HasPrefix(r.URL.Path, "/internal/relay/text-requests/") && r.Method == "GET":
					id := strings.TrimPrefix(r.URL.Path, "/internal/relay/text-requests/")
					if len(ids) == 0 || id != ids[len(ids)-1] {
						t.Error("did not query current original")
					}
					polls[id]++
					reply := platformclient.TextRecovery{Version: "oceanway.text-request.v1", RequestID: id, TaskID: "text_" + id, Protocol: "openai-chat", Fingerprint: fp, Status: "failed"}
					reply.Failure = tc.failure
					if tc.name == "historical_parameter_rejection" {
						reply.Status = "pending"
					}
					if len(ids) == tc.succeeds {
						switch polls[id] {
						case 1:
							reply.Fingerprint = "foreign-body"
						case 2:
							reply.Status = "pending"
							reply.ResponseTimedOut = true
						case 3:
							reply.Status = "not_found"
						case 4:
							reply.Version = ""
							reply.Status = "FAILURE"
						default:
							reply.Ready = true
							reply.Status = "ready"
							reply.Response = json.RawMessage(`{"choices":[{"message":{"content":"once"}}]}`)
						}
					}
					json.NewEncoder(w).Encode(map[string]any{"success": true, "data": reply})
				default:
					t.Errorf("unexpected route %s", r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			env := map[string]string{"STUDIO_PLATFORM_ORIGIN": server.URL, "STUDIO_PLATFORM_PRODUCT_KEY": "fixture.secret", "STUDIO_PUBLIC_ORIGIN": "https://studio.example.test", "STUDIO_PLATFORM_SESSION_HOSTS": "studio.example.test", "STUDIO_PLATFORM_ALLOWED_ORIGINS": "https://studio.example.test"}
			gateway, err := platformintegration.NewGatewayFromEnv(func(k string) string { return env[k] })
			if err != nil {
				t.Fatal(err)
			}
			s.studioPlatform = gateway
			req := CloudAgentRequest{Prompt: "hello", Model: "m", ChannelID: StudioPlatformChannelID, ChannelModelKey: "m", IdempotencyKey: "retry-test", PermissionMode: "read_only"}
			req.PlatformTextCapability = &platformclient.TextCapability{ContextWindowTokens: 9999999, MaxOutputTokens: 999999}
			req.Budget.MaxSteps = 1
			root, _, err := s.CreateStudioCloudAgentRun(context.Background(), "user", platformclient.SessionContext{Host: "studio.example.test", Origin: "https://studio.example.test", Cookie: "platform_session=alice"}, req, "")
			if err != nil {
				t.Fatal(err)
			}
			run, err := s.repo.CloudAgent("user", root.ID)
			if err != nil {
				t.Fatal(err)
			}
			state, err := cloudAgentDecode(run)
			if err != nil {
				t.Fatal(err)
			}
			budget := s.cloudAgentContextBudgetForRequest(state.Request)
			if !budget.Configured || budget.ContextWindowTokens != 32000 || budget.MaxOutputTokens != 4096 {
				t.Fatalf("published model capability was not frozen: %+v", budget)
			}
			err = s.enqueueCloudAgentTask(run, &state, CreateTaskRequest{Type: "canvas_text", Operation: cloudAgentStepOperation, Prompt: "hello", Model: "m", Input: map[string]any{"config": map[string]any{"channelId": StudioPlatformChannelID, "model": "m"}, "agentRequests": map[string]any{"canonical": state.Canonical}}}, nil)
			if err != nil {
				t.Fatal(err)
			}
			taskID := state.ActiveTaskID
			var task *model.Task
			for n := 1; n <= 18; n++ {
				claimStarted := time.Now()
				if err := s.ProcessNextTask(); err != nil {
					t.Fatal(err)
				}
				task, err = s.repo.Task(taskID)
				if err != nil {
					t.Fatal(err)
				}
				if cloudAgentTaskTerminal(task.Status) {
					break
				}
				if task.Stage == "平台确认本次未执行，等待重试" {
					delay := time.Second << (len(ids) - 1)
					if task.NextPollAt == nil || task.NextPollAt.Before(claimStarted.Add(delay-time.Millisecond)) || task.NextPollAt.After(time.Now().Add(delay+time.Millisecond)) {
						t.Fatalf("attempt %d: expected durable backoff %s", len(ids), delay)
					}
				}
				if n == tc.cancelAt {
					if err := s.CancelCloudAgent(context.Background(), "user", root.ID); err != nil {
						t.Fatal(err)
					}
				}
				if n == 2 {
					// Start a fresh worker with only the shared database and encryption key.
					s = &Service{repo: s.repo, dataDir: s.dataDir, studioPlatform: gateway, disablePiRuntime: true, activeCancels: make(map[string]context.CancelFunc)}
				}
				if err := db.Model(&model.Task{}).Where("id = ?", taskID).Update("next_poll_at", time.Now().Add(-time.Second)).Error; err != nil {
					t.Fatal(err)
				}
			}
			if task == nil {
				t.Fatal("missing Pi step")
			}
			if task.ProviderRequestID != "platform-original-relay" {
				t.Fatalf("missing original Platform request ID: %q", task.ProviderRequestID)
			}
			if s.cloudAgentPiTaskContextOverflow(task.ID) != (tc.name == "explicit_context_overflow") {
				t.Fatal("only a confirmed context rejection may trigger compaction")
			}
			if len(ids) != tc.wantPosts || grants != 1 || !cloudAgentTaskTerminal(task.Status) {
				t.Fatalf("posts=%d want=%d grants=%d task=%+v", len(ids), tc.wantPosts, grants, task.Status)
			}
			if tc.succeeds > 0 {
				if task.Status != model.TaskStatusSucceeded || !strings.Contains(task.ResultJSON, "once") || executions != 1 || charges != 1 {
					t.Fatalf("status=%s executions=%d charges=%d", task.Status, executions, charges)
				}
			} else if task.Status == model.TaskStatusSucceeded || executions != 0 || charges != 0 {
				t.Fatalf("status=%s executions=%d charges=%d", task.Status, executions, charges)
			}
			if tc.cancelAt > 0 {
				cancelled, err := s.repo.CloudAgent("user", root.ID)
				if err != nil {
					t.Fatal(err)
				}
				if err := s.finishCloudAgentCleanup(context.Background(), cancelled); err != nil {
					t.Fatal(err)
				}
				s.releaseCompletedStudioExecutions(context.Background())
				cancelled, err = s.repo.CloudAgent("user", root.ID)
				if err != nil || cancelled.CleanupPending || cancelled.ActiveTaskID != "" || cancelled.Status != "cancelled" {
					t.Fatalf("cancel cleanup not completed: %+v %v", cancelled, err)
				}
				authority, err := s.repo.StudioExecution("user", root.ID)
				if err != nil || !authority.Released {
					t.Fatalf("root authority not released: %+v %v", authority, err)
				}
			}
		})
	}
}
