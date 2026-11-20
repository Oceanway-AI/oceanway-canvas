package app

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"yingce/backend/internal/catalogtest"
	"yingce/backend/internal/model"
	"yingce/backend/internal/platformintegration"
)

func TestStudioPiModelStepUsesOriginalRunAuthority(t *testing.T) {
	s, db, _, _ := creationTestService(t)
	if err := db.AutoMigrate(&model.StudioExecution{}, &model.StudioMaterialReference{}); err != nil {
		t.Fatal(err)
	}
	s.legacyCloudAgentRootTask = false
	s.activeCancels = make(map[string]context.CancelFunc)
	issues, submits, releases := 0, 0, 0
	requests := map[string]string{}
	var catalogUnavailable atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case catalogtest.Route(r.URL.Path):
			if catalogUnavailable.Load() {
				w.WriteHeader(503)
				return
			}
			catalogtest.Write(t, w, r, []byte(`{"success":true,"data":{"items":[{"model_name":"pi-model","model_type":"text","invocation_status":"published_text","supported_protocols":["openai-chat"],"price":{"status":"available","version":"p1"}}]}}`))
		case r.URL.Path == "/api/platform/v1/execution-authorizations":
			issues++
			var in map[string]string
			_ = json.NewDecoder(r.Body).Decode(&in)
			_ = json.NewEncoder(w).Encode(map[string]any{"authorization_id": "exa_" + in["task_reference"], "task_reference": in["task_reference"], "product": "studio", "expires_at": time.Now().Add(time.Hour).Unix()})
		case strings.HasSuffix(r.URL.Path, "/release"):
			releases++
			w.WriteHeader(204)
		case r.URL.Path == "/internal/relay/chat/completions":
			submits++
			auth := r.Header.Get("X-Platform-Execution-Authorization")
			if auth == "" || r.Header.Get("Cookie") != "" || r.Header.Get("Origin") != "" {
				t.Error("Pi authority leaked/missing")
			}
			id := r.Header.Get("X-Platform-Text-Request-Id")
			if id == "" || requests[id] != "" {
				t.Error("missing or duplicate relay request ID")
			}
			requests[id] = auth
			_, _ = fmt.Fprintf(w, `{"choices":[{"message":{"content":"Pi platform response"}}],"usage":{"prompt_tokens":%d,"completion_tokens":1}}`, submits)
		default:
			t.Errorf("unexpected Pi request %s", r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	defer server.Close()
	env := map[string]string{"STUDIO_PLATFORM_ORIGIN": server.URL, "STUDIO_PLATFORM_PRODUCT_KEY": "fixture.secret", "STUDIO_PUBLIC_ORIGIN": "https://studio.example.test", "STUDIO_PLATFORM_SESSION_HOSTS": "studio.example.test", "STUDIO_PLATFORM_ALLOWED_ORIGINS": "https://studio.example.test"}
	gateway, err := platformintegration.NewGatewayFromEnv(func(key string) string { return env[key] })
	if err != nil {
		t.Fatal(err)
	}
	s.studioPlatform = gateway
	req := CloudAgentRequest{Prompt: "hello", Model: "pi-model", ChannelID: StudioPlatformChannelID, ChannelModelKey: "pi-model", IdempotencyKey: "studio-pi-test", PermissionMode: "read_only"}
	view, _, err := s.CreateStudioCloudAgentRun(context.Background(), "user", platformclient.SessionContext{Host: "studio.example.test", Origin: "https://studio.example.test", Cookie: "platform_session=alice"}, req, "")
	if err != nil {
		t.Fatal(err)
	}
	session := platformclient.SessionContext{Host: "studio.example.test", Origin: "https://studio.example.test", Cookie: "platform_session=alice"}
	catalogUnavailable.Store(true)
	duplicate, _, err := s.CreateStudioCloudAgentRun(context.Background(), "user", session, req, "")
	if err != nil || duplicate.ID != view.ID {
		t.Fatalf("duplicate run changed identity: %v", err)
	}
	changed := req
	changed.Prompt = "different intent"
	if _, _, err := s.CreateStudioCloudAgentRun(context.Background(), "user", session, changed, ""); err == nil {
		t.Fatal("changed intent reused original run")
	}
	catalogUnavailable.Store(false)
	if _, err := s.CloudAgentRun("other", view.ID); err == nil {
		t.Fatal("cross-account Agent access")
	}
	for turn, steps := range []int{17, 1} {
		if turn > 0 {
			req.IdempotencyKey = "studio-pi-next-turn"
			parent := view.ID
			view, _, err = s.CreateStudioCloudAgentRun(t.Context(), "user", session, req, parent)
			if err != nil || view.ID == parent || view.ParentID != parent {
				t.Fatalf("next turn: %v, %v", view, err)
			}
		}
		ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
		defer cancel()
		for step := 1; step <= steps; step++ {
			done := make(chan error, 1)
			go func() {
				_, _, err := s.runCloudAgentModelStep(ctx, "user", view.ID, []map[string]any{{"role": "user", "content": "hello"}}, "off")
				done <- err
			}()
		waitForStep:
			for {
				select {
				case err := <-done:
					if err != nil {
						t.Fatal(err)
					}
					break waitForStep
				case <-time.After(20 * time.Millisecond):
					if err := s.ProcessNextTask(); err != nil {
						t.Fatal(err)
					}
				}
			}
			_, state := agentInterjectionState(t, s, view.ID)
			id := state.TaskIDs[len(state.TaskIDs)-1]
			task, err := s.repo.Task(id)
			if err != nil || task.Status != model.TaskStatusSucceeded || task.AgentRunID != view.ID || !strings.Contains(task.ResultJSON, "Pi platform response") {
				t.Fatalf("turn=%d step=%d task=%v err=%v", turn, step, task, err)
			}
			var result struct {
				Usage map[string]int `json:"usage"`
			}
			if err := json.Unmarshal([]byte(task.ResultJSON), &result); err != nil || result.Usage["totalTokens"] != submits+1 ||
				state.Step != step || requests["studio-"+id] != "exa_"+view.ID || s.cloudAgentModelTaskRetryable(id) {
				t.Fatalf("incorrect usage, authority or step identity: turn=%d step=%d result=%s", turn, step, task.ResultJSON)
			}
			if _, err := s.handleMessageEnd("user", view.ID, map[string]any{"role": "assistant"}); err != nil {
				t.Fatal(err)
			}
		}
		if err := s.completeCloudAgentPiRun("user", view.ID); err != nil {
			t.Fatal(err)
		}
		s.releaseCompletedStudioExecutions(t.Context())
	}
	if submits != 18 || issues != 2 || releases != 2 || len(requests) != 18 {
		t.Fatalf("unexpected relay lifecycle: submits=%d issues=%d releases=%d", submits, issues, releases)
	}
	if db.Migrator().HasTable("billing_orders") {
		t.Fatal("Pi created local bill")
	}
}

func TestStudioPiRuntimeToolRoundTrip(t *testing.T) {
	testStudioPiRuntimeToolRoundTrip(t, false)
}

func TestStudioPiRuntimeMediaApprovalRoundTrip(t *testing.T) {
	testStudioPiRuntimeToolRoundTrip(t, true)
}

func testStudioPiRuntimeToolRoundTrip(t *testing.T, media bool) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Fatal("native Pi acceptance requires node")
	}
	if _, err := agentRuntimeDirForTest(); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var bodies []map[string]any
	issues, releases := 0, 0
	mediaSubmits, mediaRecovers, quotes := 0, 0, 0
	var rejectedID, rejectedFingerprint string
	rejectionReads := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		mu.Lock()
		defer mu.Unlock()
		switch {
		case catalogtest.Route(r.URL.Path):
			if media {
				catalogtest.Write(t, w, r, []byte(studioAgentMediaFixtureCatalog))
			} else {
				catalogtest.Write(t, w, r, []byte(`{"success":true,"data":{"items":[{"model_name":"pi-model","model_type":"text","invocation_status":"published_text","supported_protocols":["openai-chat"],"price":{"status":"available","version":"p1"}}]}}`))
			}
		case r.URL.Path == "/api/platform/v1/execution-authorizations":
			issues++
			var in map[string]string
			_ = json.NewDecoder(r.Body).Decode(&in)
			_ = json.NewEncoder(w).Encode(map[string]any{"authorization_id": "exa_native_pi", "task_reference": in["task_reference"], "product": "studio", "expires_at": time.Now().Add(time.Hour).Unix()})
		case strings.HasSuffix(r.URL.Path, "/release"):
			releases++
			w.WriteHeader(204)
		case strings.HasSuffix(r.URL.Path, "/media/consumption-preview"):
			quotes++
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": platformclient.ConsumptionPreview{CapabilityRevision: strings.Repeat("a", 64), ConnectionRevision: 7, Model: "image-model", ModelType: "image", PriceVersion: "price-1", CombinationSupported: true, Allowed: true, WalletPoints: "0.123456789"}})
		case r.URL.Path == "/internal/relay/images/generations":
			mediaSubmits++
			raw, _ := base64.RawURLEncoding.DecodeString(r.Header.Get("X-Platform-Media-Consumption"))
			var cap platformclient.MediaConsumptionConstraints
			if json.Unmarshal(raw, &cap) != nil || cap.ExpectedCapabilityRevision != strings.Repeat("a", 64) || cap.ExpectedConnectionRevision != 7 || cap.ExpectedPriceVersion != "price-1" || cap.WalletMaxPoints != "" {
				t.Errorf("native Pi lost frozen consumption: %s", raw)
			}
			w.WriteHeader(503)
		case strings.HasPrefix(r.URL.Path, "/internal/relay/media-requests/"):
			mediaRecovers++
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": platformclient.MediaRecovery{Version: "oceanway.media-request.v1", RequestID: strings.TrimPrefix(r.URL.Path, "/internal/relay/media-requests/"), Status: "FAILURE", NoExecution: true}})
		case rejectedID != "" && r.URL.Path == "/internal/relay/text-requests/"+rejectedID:
			rejectionReads++
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": platformclient.TextRecovery{Version: "oceanway.text-request.v1", RequestID: rejectedID, TaskID: "text_fixture", Protocol: "openai-chat", Fingerprint: rejectedFingerprint, Status: "failed"}})
		case r.URL.Path == "/internal/relay/chat/completions":
			if r.Header.Get("X-Platform-Execution-Authorization") != "exa_native_pi" || r.Header.Get("Cookie") != "" {
				t.Error("incorrect Pi worker authority")
			}
			raw, _ := io.ReadAll(r.Body)
			requestID := r.Header.Get("X-Platform-Text-Request-Id")
			if !media && rejectedID == "" {
				rejectedID = requestID
				rejectedFingerprint = fmt.Sprintf("%x", sha256.Sum256(append([]byte("openai-chat\x00"), raw...)))
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			if !media && (requestID == rejectedID || rejectionReads != 1) {
				t.Error("Pi submitted without definitive recovery of the previous request")
			}
			var body map[string]any
			_ = json.Unmarshal(raw, &body)
			bodies = append(bodies, body)
			if len(bodies) == 1 && media {
				args, _ := json.Marshal(map[string]any{"mode": "image", "channelId": StudioPlatformChannelID, "channelModelKey": "image-model", "prompt": "a green square", "size": "1:1", "title": "图片草稿", "referenceNodeIds": []string{}})
				_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": "", "tool_calls": []any{map[string]any{"id": "create-image", "type": "function", "function": map[string]any{"name": "generate_media", "arguments": string(args)}}}}}}})
			} else if len(bodies) == 1 {
				_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"","tool_calls":[{"id":"read-canvas","type":"function","function":{"name":"canvas_get_state","arguments":"{}"}}]}}]}`))
			} else {
				_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"画布已检查完毕"}}]}`))
			}
		default:
			t.Errorf("unexpected native Pi path: %s", r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	defer server.Close()
	base, db, _, _ := creationTestService(t)
	if err := db.AutoMigrate(&model.StudioExecution{}, &model.StudioMaterialReference{}); err != nil {
		t.Fatal(err)
	}
	s := New(base.repo, base.dataDir)
	t.Cleanup(func() { _ = s.Close() })
	env := map[string]string{"STUDIO_PLATFORM_ORIGIN": server.URL, "STUDIO_PLATFORM_PRODUCT_KEY": "fixture.secret", "STUDIO_PUBLIC_ORIGIN": "https://studio.example.test", "STUDIO_PLATFORM_SESSION_HOSTS": "studio.example.test", "STUDIO_PLATFORM_ALLOWED_ORIGINS": "https://studio.example.test"}
	gateway, err := platformintegration.NewGatewayFromEnv(func(key string) string { return env[key] })
	if err != nil {
		t.Fatal(err)
	}
	s.studioPlatform = gateway
	if err := db.Create(&model.CanvasProject{ID: "studio-pi-canvas", UserID: "user", PayloadJSON: `{"nodes":[],"connections":[]}`}).Error; err != nil {
		t.Fatal(err)
	}
	req := CloudAgentRequest{CanvasID: "studio-pi-canvas", Prompt: "检查画布", Model: "pi-model", ChannelID: StudioPlatformChannelID, ChannelModelKey: "pi-model", IdempotencyKey: "studio-pi-native-test", PermissionMode: "read_only", ContextScope: []string{"canvas"}}
	req.Budget.MaxSteps = 4
	if media {
		req.PermissionMode = "request_approval"
	}
	view, _, err := s.CreateStudioCloudAgentRun(context.Background(), "user", platformclient.SessionContext{Host: "studio.example.test", Origin: "https://studio.example.test", Cookie: "platform_session=alice"}, req, "")
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(40 * time.Second)
	approved := false
	for time.Now().Before(deadline) {
		if media {
			db.Model(&model.Task{}).Where("agent_run_id = ? AND type = ?", view.ID, "canvas_image").Update("next_poll_at", time.Now().Add(-time.Second))
		}
		if err := s.ProcessNextTask(); err != nil {
			t.Fatal(err)
		}
		run, err := s.repo.CloudAgent("user", view.ID)
		if err != nil {
			t.Fatal(err)
		}
		if media && run.Status == "waiting_approval" && !approved {
			state, err := cloudAgentDecode(run)
			if err != nil {
				t.Fatal(err)
			}
			quote, _, err := s.QuoteStudioAgentMedia(context.Background(), "user", platformclient.SessionContext{Host: "studio.example.test", Origin: "https://studio.example.test", Cookie: "session=fixture"}, run.ID, state.Approval.ID, true)
			if err != nil {
				t.Fatal(err)
			}
			if err := s.DecideStudioAgentMedia("user", run.ID, state.Approval.ID, "approve", "", quote.ID, nil); err != nil {
				t.Fatal(err)
			}
			approved = true
		}
		if cloudAgentRunTerminal(run.Status) {
			if run.Status != "completed" {
				t.Fatalf("native Pi failed: %s %s", run.Status, run.FailureMessage)
			}
			s.releaseCompletedStudioExecutions(context.Background())
			mu.Lock()
			defer mu.Unlock()
			if len(bodies) != 2 || issues != 1 || releases != 1 {
				t.Fatalf("calls=%d issues=%d releases=%d", len(bodies), issues, releases)
			}
			raw, _ := json.Marshal(bodies[1]["messages"])
			if !media && (!strings.Contains(string(raw), `"tool_call_id":"read-canvas"`) || strings.Contains(string(raw), `\"error\"`)) {
				t.Fatalf("tool receipt missing: %s", raw)
			}
			if media && (!approved || mediaSubmits != 1 || mediaRecovers != 1 || quotes != 1 || !strings.Contains(string(raw), "create-image")) {
				t.Fatalf("native media approval not resumed exactly once: submits=%d recoveries=%d quotes=%d", mediaSubmits, mediaRecovers, quotes)
			}
			if session, err := s.repo.CloudAgentPiSession("user", view.ID); err != nil || session.SessionJSONL == "" {
				t.Fatalf("Pi snapshot missing: %v", err)
			}
			return
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatal("native Pi did not finish")
}
