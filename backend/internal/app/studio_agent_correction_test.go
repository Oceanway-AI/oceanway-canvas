package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"yingce/backend/internal/catalogtest"
	"yingce/backend/internal/model"
	"yingce/backend/internal/platformintegration"
)

func TestStudioAgentSuccessfulOutputCorrection(t *testing.T) {
	for _, scenario := range []struct {
		name        string
		invalidArgs bool
		bounded     bool
		wantPosts   int
	}{
		{name: "tool_limit_corrected", wantPosts: 2},
		{name: "tool_limit_bounded", bounded: true, wantPosts: 4},
		{name: "invalid_arguments", invalidArgs: true, wantPosts: 2},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			s, db, _, _ := creationTestService(t)
			if err := db.AutoMigrate(&model.StudioExecution{}, &model.StudioMaterialReference{}); err != nil {
				t.Fatal(err)
			}
			if err := db.Create(&model.CanvasProject{ID: "agent-canvas", UserID: "user", PayloadJSON: `{"nodes":[],"connections":[]}`}).Error; err != nil {
				t.Fatal(err)
			}
			s.legacyCloudAgentRootTask = false
			s.activeCancels = make(map[string]context.CancelFunc)
			s.cloudAgentModelRetryBase = time.Millisecond
			var posts atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/api/platform/v1/products/studio/models", "/api/platform/v1/products/studio/model-catalog/summaries", "/api/platform/v1/products/studio/model-catalog/detail":
					catalogtest.Write(t, w, r, []byte(`{"success":true,"data":{"items":[{"model_name":"m","model_type":"text","invocation_status":"published_text","supported_protocols":["openai-chat"],"price":{"status":"available","version":"p1"}}]}}`))
				case "/api/platform/v1/execution-authorizations":
					var request map[string]string
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
						t.Error(err)
					}
					json.NewEncoder(w).Encode(map[string]any{"authorization_id": "exa_correction", "product": "studio", "task_reference": request["task_reference"], "expires_at": time.Now().Add(time.Hour).Unix()})
				case "/internal/relay/chat/completions":
					attempt := int(posts.Add(1))
					body, err := io.ReadAll(r.Body)
					if err != nil {
						t.Error(err)
					}
					if r.Header.Get("X-Platform-Execution-Authorization") != "exa_correction" || r.Header.Get("X-Platform-Text-Request-Id") == "" {
						t.Error("correction lost the original execution authority or step identity")
					}
					if attempt > 1 {
						marker := "invalid_output"
						if scenario.invalidArgs {
							marker = "fix_arguments"
						}
						if !strings.Contains(string(body), marker) {
							t.Errorf("correction request omitted %s", marker)
						}
					}
					message := map[string]any{"content": "complete"}
					if attempt == 1 || scenario.bounded {
						count := cloudAgentMaxToolCalls + 1
						arguments := "{}"
						if scenario.invalidArgs {
							count, arguments = 1, `{"ops":`
						}
						calls := make([]map[string]any, count)
						for i := range calls {
							calls[i] = map[string]any{"id": fmt.Sprintf("call-%d-%d", attempt, i), "type": "function", "function": map[string]any{"name": "canvas_get_state", "arguments": arguments}}
						}
						message = map[string]any{"content": "", "tool_calls": calls}
					}
					w.Header().Set("X-Oneapi-Request-Id", fmt.Sprintf("platform-correction-%d", attempt))
					json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": message}}, "usage": map[string]any{"prompt_tokens": 100, "completion_tokens": 10, "prompt_tokens_details": map[string]int{"cached_tokens": 20}}})
				default:
					t.Errorf("unexpected request %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			env := map[string]string{"STUDIO_PLATFORM_ORIGIN": server.URL, "STUDIO_PLATFORM_PRODUCT_KEY": "fixture.secret", "STUDIO_PUBLIC_ORIGIN": "https://studio.example.test", "STUDIO_PLATFORM_SESSION_HOSTS": "studio.example.test", "STUDIO_PLATFORM_ALLOWED_ORIGINS": "https://studio.example.test"}
			var err error
			s.studioPlatform, err = platformintegration.NewGatewayFromEnv(func(key string) string { return env[key] })
			if err != nil {
				t.Fatal(err)
			}
			req := CloudAgentRequest{CanvasID: "agent-canvas", ContextScope: []string{"canvas"}, Prompt: "hello", Model: "m", ChannelID: StudioPlatformChannelID, ChannelModelKey: "m", IdempotencyKey: "correction-test", PermissionMode: "read_only"}
			req.Budget.MaxSteps = 8
			root, _, err := s.CreateStudioCloudAgentRun(t.Context(), "user", platformclient.SessionContext{Host: "studio.example.test", Origin: "https://studio.example.test", Cookie: "platform_session=fixture"}, req, "")
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			workerDone := make(chan struct{})
			go func() {
				defer close(workerDone)
				ticker := time.NewTicker(5 * time.Millisecond)
				defer ticker.Stop()
				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						if err := s.ProcessNextTask(); err != nil {
							t.Error(err)
							return
						}
					}
				}
			}()
			defer func() { cancel(); <-workerDone }()
			payload := map[string]json.RawMessage{"messages": json.RawMessage(`[{"role":"user","content":"hello"}]`), "thinkingLevel": json.RawMessage(`"off"`)}
			result, modelErr := s.cloudAgentPiModel(ctx, "user", root.ID, payload)
			if scenario.invalidArgs {
				if modelErr != nil {
					t.Fatal(modelErr)
				}
				calls := result.(map[string]any)["toolCalls"].([]map[string]any)
				if len(calls) != 1 {
					t.Fatalf("missing runtime tool call: %+v", calls)
				}
				arguments, err := json.Marshal(calls[0]["arguments"])
				if err != nil {
					t.Fatal(err)
				}
				tool, err := s.cloudAgentPiTool(ctx, "user", root.ID, map[string]json.RawMessage{"callId": json.RawMessage(`"call-1-0"`), "name": json.RawMessage(`"canvas_get_state"`), "arguments": arguments})
				if err != nil {
					t.Fatal(err)
				}
				receipt, _ := json.Marshal(tool)
				if !strings.Contains(string(receipt), "fix_arguments") || posts.Load() != 1 {
					t.Fatalf("invalid tool did not replay its correction receipt: %s", receipt)
				}
				result, modelErr = s.cloudAgentPiModel(ctx, "user", root.ID, payload)
			}
			if scenario.bounded {
				if modelErr == nil || !strings.Contains(modelErr.Error(), "工具调用") {
					t.Fatalf("output limit did not stop after bounded correction: %v", modelErr)
				}
			} else {
				if modelErr != nil {
					t.Fatal(modelErr)
				}
				delivered := result.(map[string]any)
				usage, _ := delivered["usage"].(map[string]int64)
				if delivered["text"] != "complete" || usage["input"] != 80 || usage["cacheRead"] != 20 || usage["totalTokens"] != 110 {
					t.Fatalf("successful correction lost text or original usage: %+v", delivered)
				}
			}
			run, err := s.repo.CloudAgent("user", root.ID)
			if err != nil {
				t.Fatal(err)
			}
			state, err := cloudAgentDecode(run)
			if err != nil || state.ActiveTaskID != "" || len(state.Calls) != 0 || state.InvalidArgumentSteps != 0 {
				t.Fatalf("correction left an active task or invalid calls: %+v %v", state, err)
			}
			var steps []model.Task
			if err := db.Where("agent_run_id = ?", root.ID).Find(&steps).Error; err != nil {
				t.Fatal(err)
			}
			if len(steps) != scenario.wantPosts || int(posts.Load()) != scenario.wantPosts {
				t.Fatalf("steps=%d posts=%d expected=%d", len(steps), posts.Load(), scenario.wantPosts)
			}
			for _, step := range steps {
				if step.Status != model.TaskStatusSucceeded || !strings.HasPrefix(step.ProviderRequestID, "platform-correction-") {
					t.Fatalf("paid model output lost its successful step record: %+v", step)
				}
			}
		})
	}
}
