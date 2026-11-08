package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/models"):
			_, _ = w.Write([]byte(`{"success":true,"data":{"items":[{"model_name":"pi-model","model_type":"text","invocation_status":"published_text","supported_protocols":["openai-chat"],"price":{"status":"available","version":"p1"}}]}}`))
		case r.URL.Path == "/api/platform/v1/execution-authorizations":
			issues++
			var in map[string]string
			_ = json.NewDecoder(r.Body).Decode(&in)
			_ = json.NewEncoder(w).Encode(map[string]any{"authorization_id": "exa_pi_run", "task_reference": in["task_reference"], "product": "studio", "expires_at": time.Now().Add(time.Hour).Unix()})
		case strings.HasSuffix(r.URL.Path, "/release"):
			releases++
			w.WriteHeader(204)
		case r.URL.Path == "/internal/relay/chat/completions":
			submits++
			if r.Header.Get("X-Platform-Execution-Authorization") != "exa_pi_run" || r.Header.Get("Cookie") != "" || r.Header.Get("Origin") != "" {
				t.Error("Pi authority leaked/missing")
			}
			if r.Header.Get("X-Platform-Text-Request-Id") == "" {
				t.Error("Pi request not durable")
			}
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Pi platform response"}}]}`))
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
	req.Budget.MaxSteps = 4
	view, _, err := s.CreateStudioCloudAgentRun(context.Background(), "user", platformclient.SessionContext{Host: "studio.example.test", Origin: "https://studio.example.test", Cookie: "platform_session=alice"}, req, "")
	if err != nil {
		t.Fatal(err)
	}
	session := platformclient.SessionContext{Host: "studio.example.test", Origin: "https://studio.example.test", Cookie: "platform_session=alice"}
	duplicate, _, err := s.CreateStudioCloudAgentRun(context.Background(), "user", session, req, "")
	if err != nil || duplicate.ID != view.ID {
		t.Fatalf("duplicate run changed identity: %v", err)
	}
	changed := req
	changed.Prompt = "different intent"
	if _, _, err := s.CreateStudioCloudAgentRun(context.Background(), "user", session, changed, ""); err == nil {
		t.Fatal("changed intent reused original run")
	}
	if _, err := s.CloudAgentRun("other", view.ID); err == nil {
		t.Fatal("cross-account Agent access")
	}
	run, err := s.repo.CloudAgent("user", view.ID)
	if err != nil {
		t.Fatal(err)
	}
	state, err := cloudAgentDecode(run)
	if err != nil {
		t.Fatal(err)
	}
	step := CreateTaskRequest{Type: "canvas_text", Operation: cloudAgentStepOperation, Prompt: "hello", Model: "pi-model", Input: map[string]any{"config": map[string]any{"channelId": StudioPlatformChannelID, "model": "pi-model"}, "agentRequests": map[string]any{"canonical": state.Canonical}}}
	if err := s.enqueueCloudAgentTask(run, &state, step, nil); err != nil {
		t.Fatal(err)
	}
	id := state.ActiveTaskID
	if id == "" || id == view.ID {
		t.Fatal("Pi model step did not create separate durable task")
	}
	if err := s.ProcessNextTask(); err != nil {
		t.Fatal(err)
	}
	task, err := s.repo.Task(id)
	if err != nil {
		t.Fatal(err)
	}
	if task.Status != model.TaskStatusSucceeded || !strings.Contains(task.ResultJSON, "Pi platform response") {
		t.Fatalf("Pi result missing: %#v", task)
	}
	if submits != 1 || issues != 1 || releases != 0 {
		t.Fatalf("step replaced/released run authority: submits=%d issues=%d releases=%d", submits, issues, releases)
	}
	if s.cloudAgentModelTaskRetryable(id) {
		t.Fatal("Platform step entered old auto retry")
	}
	var count int64
	db.Model(&model.BillingOrder{}).Count(&count)
	if count != 0 {
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
	t.Setenv("REDIS_URL", "")
	var mu sync.Mutex
	var bodies []map[string]any
	issues, releases := 0, 0
	mediaSubmits, mediaRecovers, quotes := 0, 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		mu.Lock()
		defer mu.Unlock()
		switch {
		case strings.HasSuffix(r.URL.Path, "/models"):
			if media {
				_, _ = w.Write([]byte(studioAgentMediaFixtureCatalog))
			} else {
				_, _ = w.Write([]byte(`{"success":true,"data":{"items":[{"model_name":"pi-model","model_type":"text","invocation_status":"published_text","supported_protocols":["openai-chat"],"price":{"status":"available","version":"p1"}}]}}`))
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
			if json.Unmarshal(raw, &cap) != nil || cap.ExpectedCapabilityRevision != strings.Repeat("a", 64) || cap.ExpectedConnectionRevision != 7 || cap.ExpectedPriceVersion != "price-1" || cap.WalletMaxPoints != "0.123456789" {
				t.Errorf("native Pi lost frozen consumption: %s", raw)
			}
			w.WriteHeader(503)
		case strings.HasPrefix(r.URL.Path, "/internal/relay/media-requests/"):
			mediaRecovers++
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": platformclient.MediaRecovery{RequestID: strings.TrimPrefix(r.URL.Path, "/internal/relay/media-requests/"), Status: "FAILURE", NoExecution: true}})
		case r.URL.Path == "/internal/relay/chat/completions":
			if r.Header.Get("X-Platform-Execution-Authorization") != "exa_native_pi" || r.Header.Get("Cookie") != "" {
				t.Error("incorrect Pi worker authority")
			}
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
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
			quote, _, err := s.QuoteStudioAgentMedia(context.Background(), "user", platformclient.SessionContext{Host: "studio.example.test", Origin: "https://studio.example.test", Cookie: "session=fixture"}, run.ID, state.Approval.ID, "0.123456789")
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
