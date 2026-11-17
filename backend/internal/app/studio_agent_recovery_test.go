package app

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"yingce/backend/internal/model"
	"yingce/backend/internal/platformintegration"
)

func TestStudioTextUsageNativeBuckets(t *testing.T) {
	for _, tc := range []struct {
		protocol platformclient.TextProtocol
		body     string
	}{
		{platformclient.Responses, `{"usage":{"input_tokens":120,"output_tokens":10,"input_tokens_details":{"cached_tokens":20}}}`},
		{platformclient.ChatCompletions, `{"usage":{"prompt_tokens":120,"completion_tokens":10,"prompt_tokens_details":{"cached_tokens":20}}}`},
		{platformclient.Messages, `{"usage":{"input_tokens":100,"output_tokens":10,"cache_read_input_tokens":20,"cache_creation_input_tokens":0}}`},
	} {
		usage := studioTextUsage([]byte(tc.body), tc.protocol)
		if usage["input"] != 100 || usage["cacheRead"] != 20 || usage["output"] != 10 || usage["totalTokens"] != 130 {
			t.Fatalf("%s: %+v", tc.protocol, usage)
		}
	}
	if studioTextUsage([]byte(`{"usage":{"input_tokens":1,"output_tokens":-1}}`), platformclient.Responses) != nil || studioTextUsage([]byte(`{}`), platformclient.Responses) != nil {
		t.Fatal("invalid or missing usage must remain unavailable")
	}
}

func TestStudioAgentTaskProjectionAndCleanup(t *testing.T) {
	s, db, _, _ := creationTestService(t)
	if err := db.AutoMigrate(&model.StudioExecution{}); err != nil {
		t.Fatal(err)
	}
	s.studioPlatform = &platformintegration.Gateway{}
	root := model.Task{ID: "root", UserID: "user", Operation: cloudAgentOperation, Provider: "platform", Type: "canvas_text", Status: model.TaskStatusTextReplay, InputJSON: `{}`}
	child := model.Task{ID: "child", UserID: "user", AgentRunID: root.ID, Provider: "platform", Type: "canvas_text", Status: model.TaskStatusFailed, Error: "模型请求参数无效，请检查输入。", InputJSON: `{}`}
	for _, task := range []*model.Task{&root, &child} {
		if err := db.Create(task).Error; err != nil {
			t.Fatal(err)
		}
	}
	run := model.CloudAgentExecution{ID: root.ID, UserID: "user", Status: "waiting_approval", StateJSON: `{}`}
	if err := db.Create(&run).Error; err != nil {
		t.Fatal(err)
	}
	active, err := s.TasksWithOptions("user", TaskListOptions{ActiveOnly: true})
	if err != nil || len(active) != 1 || active[0].AgentRunID != root.ID || active[0].Stage != "等待审批" {
		t.Fatalf("active=%+v err=%v", active, err)
	}
	if err := db.Model(&run).Updates(map[string]any{"status": "cancelled", "cleanup_pending": true, "state_json": "broken"}).Error; err != nil {
		t.Fatal(err)
	}
	detail, err := s.Task("user", root.ID)
	if err != nil || detail.Status != model.TaskStatusCancelled || !detail.AgentCleanupPending {
		t.Fatalf("detail=%+v err=%v", detail, err)
	}
	if _, err := s.Task("other", root.ID); err == nil {
		t.Fatal("foreign root exposed")
	}
	rows, err := s.Tasks("user", 20)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.ID == child.ID && (row.AgentRunID != root.ID || row.Error == "") {
			t.Fatalf("step relation/error missing: %+v", row)
		}
	}
	stored, err := s.repo.Task(root.ID)
	if err != nil || stored.Status != model.TaskStatusTextReplay {
		t.Fatal("read projection changed carrier storage")
	}
	if err := s.runCloudAgentPiSession(context.Background(), "user", root.ID); err != nil {
		t.Fatal(err)
	}
	final, err := s.repo.CloudAgent("user", root.ID)
	if err != nil || final.CleanupPending || final.Status != "cancelled" {
		t.Fatalf("corrupt terminal snapshot did not finish cleanup: %+v %v", final, err)
	}
}

func TestStudioAgentVisionVerifiedPixels(t *testing.T) {
	s, db, _, _ := creationTestService(t)
	if err := db.AutoMigrate(&model.StudioMaterialReference{}); err != nil {
		t.Fatal(err)
	}
	pixels, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+a5l8AAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	reads, accessCalls := 0, 0
	files := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reads++
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Error("credentials forwarded to material URL")
		}
		w.Write(pixels)
	}))
	defer files.Close()
	t.Setenv("CANVAS_ALLOWED_PRIVATE_UPSTREAM_HOSTS", "127.0.0.1")
	transport := OutboundHTTPClient(time.Second).Transport.(*http.Transport)
	originalTLS := transport.TLSClientConfig
	transport.TLSClientConfig = &tls.Config{RootCAs: files.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs}
	t.Cleanup(func() { transport.CloseIdleConnections(); transport.TLSClientConfig = originalTLS })
	expired := false
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accessCalls++
		if r.URL.Path != "/internal/materials/access" || r.Header.Get("X-Platform-Execution-Authorization") != "exa_vision" {
			t.Errorf("unexpected access authority %s", r.URL.Path)
		}
		var request platformclient.AccessRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if len(request.Items) != 1 || request.Items[0].AssetID != "asset" || request.Items[0].VersionID != "version" || request.Items[0].BindingID != "binding" || request.Items[0].Purpose != "download" || request.Items[0].Variant != "original" {
			t.Errorf("lost fixed binding: %+v", request)
		}
		expires := time.Now().Add(time.Hour)
		if expired {
			expires = time.Now().Add(-time.Hour)
		}
		json.NewEncoder(w).Encode(platformclient.AccessResponse{Protocol: platformclient.MaterialsProtocol, Results: []platformclient.AccessResult{{ItemID: "image", Status: "ready", AssetID: "asset", VersionID: "version", FileID: "file", RequestedVariant: "original", ActualVariant: "original", URL: files.URL, ExpiresAt: expires.Format(time.RFC3339), RefreshAt: expires.Add(-time.Minute).Format(time.RFC3339), Revision: "v1"}}})
	}))
	defer api.Close()
	env := map[string]string{"STUDIO_PLATFORM_ORIGIN": api.URL, "STUDIO_PLATFORM_PRODUCT_KEY": "fixture.secret", "STUDIO_PUBLIC_ORIGIN": "https://studio.example.test", "STUDIO_PLATFORM_SESSION_HOSTS": "studio.example.test", "STUDIO_PLATFORM_ALLOWED_ORIGINS": "https://studio.example.test"}
	s.studioPlatform, err = platformintegration.NewGatewayFromEnv(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	resource := model.Resource{ID: "image", UserID: "user", Status: model.ResourceStatusReady, Provider: "platform", Kind: "image", MimeType: "image/png", Size: int64(len(pixels))}
	pin := model.StudioMaterialReference{ResourceID: "image", UserID: "user", AssetID: "asset", VersionID: "version", FileID: "file", BindingID: "binding", SHA256: fmt.Sprintf("%x", sha256.Sum256(pixels))}
	if err := db.Create(&resource).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&pin).Error; err != nil {
		t.Fatal(err)
	}
	req := CreateTaskRequest{Type: "canvas_text", Prompt: "describe", Input: map[string]any{
		"config":          map[string]any{"channelId": StudioPlatformChannelID},
		"referenceImages": []any{map[string]any{"storageKey": "resource:image"}},
		"agentRequests": map[string]any{"canonical": canonicalAgentRequest{
			ToolChoice: "auto",
			Messages: []map[string]any{{"role": "user", "content": []any{
				map[string]any{"type": "image_url", "image_url": map[string]any{"url": "resource:image"}},
			}}},
		}},
	}}
	selected := platformintegration.ModelSelection{Name: "vision", Kind: "text", TextCapability: &platformclient.TextCapability{ContextWindowTokens: 32000, MaxOutputTokens: 4096, MaxImages: 1, MaxImageBytes: 1048576}}
	materials := s.studioPlatform.Client.WorkerMaterials(platformclient.MaterialContext{ExecutionAuthorization: "exa_vision"})
	for _, protocol := range []platformclient.TextProtocol{platformclient.ChatCompletions, platformclient.Responses, platformclient.Messages} {
		selected.TextProtocol = protocol
		body, err := s.studioTextBodyWithMaterials(context.Background(), "user", materials, req, selected)
		if err != nil || !strings.Contains(string(body), base64.StdEncoding.EncodeToString(pixels)) || strings.Contains(string(body), "resource:image") || strings.Contains(string(body), files.URL) {
			t.Fatalf("%s pixels absent or unsafe body: %v", protocol, err)
		}
	}
	if reads != 3 || accessCalls != 3 {
		t.Fatal("vision did not resolve original pixels")
	}
	ordinary := req
	ordinary.Input = map[string]any{"config": req.Input["config"], "referenceImages": req.Input["referenceImages"]}
	for _, protocol := range []platformclient.TextProtocol{platformclient.ChatCompletions, platformclient.Responses, platformclient.Messages} {
		selected.TextProtocol = protocol
		body, err := s.studioTextBodyWithMaterials(context.Background(), "user", materials, ordinary, selected)
		if err != nil || !strings.Contains(string(body), base64.StdEncoding.EncodeToString(pixels)) {
			t.Fatalf("ordinary text %s lost pixels: %v", protocol, err)
		}
	}
	if _, err := s.studioTextBodyWithMaterials(context.Background(), "other", materials, req, selected); err == nil {
		t.Fatal("foreign image accepted")
	}
	expired = true
	if _, err := s.studioTextBodyWithMaterials(context.Background(), "user", materials, req, selected); err == nil {
		t.Fatal("expired image ticket accepted")
	}
	if reads != 6 {
		t.Fatal("invalid authority fetched pixels")
	}
	expired = false
	selected.TextCapability.MaxImages = 0
	if _, err := s.studioTextBodyWithMaterials(context.Background(), "user", materials, req, selected); err == nil {
		t.Fatal("text-only model accepted image input")
	}
	selected.TextCapability.MaxImages, selected.TextCapability.MaxImageBytes = 1, 10
	if _, err := s.studioTextBodyWithMaterials(context.Background(), "user", materials, req, selected); err == nil {
		t.Fatal("model image byte limit ignored")
	}
}

func TestStudioAgentDeclaredOutputLimit(t *testing.T) {
	selected := platformintegration.ModelSelection{Name: "large-context", Kind: "text", TextProtocol: platformclient.Responses, TextCapability: &platformclient.TextCapability{ContextWindowTokens: 1000000, MaxOutputTokens: 200000}}
	req := CreateTaskRequest{Type: "canvas_text", Prompt: "fixture", Input: map[string]any{"config": map[string]any{"channelId": StudioPlatformChannelID}, "textOptions": map[string]any{"maxOutputTokens": 150000}}}
	if _, err := studioTextBody(req, selected); err != nil {
		t.Fatalf("published output limit rejected: %v", err)
	}
	req.Input["textOptions"] = map[string]any{"maxOutputTokens": 200001}
	if _, err := studioTextBody(req, selected); err == nil {
		t.Fatal("published output limit exceeded")
	}
}
