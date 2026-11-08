package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"yingce/backend/internal/model"
	"yingce/backend/internal/platformintegration"
	"yingce/backend/internal/repository"
)

const studioAgentMediaFixtureCatalog = `{"success":true,"data":{"items":[{"model_name":"pi-model","model_type":"text","invocation_status":"published_text","supported_protocols":["openai-chat"],"price":{"status":"available","version":"p1"}},{"model_name":"image-model","model_type":"image","invocation_status":"available","price":{"status":"available","version":"price-1"},"media_generation_profile":{"default_variant":"generate","variants":[{"id":"generate","protocol":"openai-images","operation":"generate","method":"POST","path":"/images/generations","defaults":{"n":1,"size":"1:1"},"specifications_schema":{"type":"object","additionalProperties":false,"properties":{"n":{"type":"integer","enum":[1]},"size":{"type":"string","enum":["1:1"]}},"required":["n","size"]},"parameter_bindings":{"n":"n","size":"size"}}]}}]}}`

func TestStudioPiMediaApprovalFreezesQuoteAndSharesAuthority(t *testing.T) {
	for _, scenario := range []string{"approve", "expired", "changed_canvas", "reject", "stopped"} {
		t.Run(scenario, func(t *testing.T) { studioPiMediaApprovalScenario(t, scenario) })
	}
}

func studioPiMediaApprovalScenario(t *testing.T, scenario string) {
	s, db, _, _ := creationTestService(t)
	if err := db.AutoMigrate(&model.StudioExecution{}, &model.StudioMaterialReference{}); err != nil {
		t.Fatal(err)
	}
	s.legacyCloudAgentRootTask = false
	s.activeCancels = make(map[string]context.CancelFunc)
	quotes, issues := 0, 0
	submits, recoveries := 0, 0
	var quotedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/models"):
			_, _ = w.Write([]byte(studioAgentMediaFixtureCatalog))
		case r.URL.Path == "/api/platform/v1/execution-authorizations":
			issues++
			var in map[string]string
			_ = json.NewDecoder(r.Body).Decode(&in)
			_ = json.NewEncoder(w).Encode(map[string]any{"authorization_id": "exa_media_run", "task_reference": in["task_reference"], "product": "studio", "expires_at": time.Now().Add(time.Hour).Unix()})
		case strings.HasSuffix(r.URL.Path, "/media/consumption-preview"):
			quotes++
			var preview platformclient.PreviewRequest
			_ = json.NewDecoder(r.Body).Decode(&preview)
			quotedBody = preview.Request.NativeRequest
			if r.Header.Get("Origin") != "https://studio.example.test" || r.Header.Get("Cookie") == "" {
				t.Error("quote lost genuine browser authority")
			}
			_, _ = w.Write([]byte(`{"success":true,"data":{"model":"image-model","model_type":"image","price_version":"price-1","combination_supported":true,"allowed":true,"wallet_points":"0.123456789"}}`))
		case r.URL.Path == "/internal/relay/images/generations":
			submits++
			body, _ := io.ReadAll(r.Body)
			if string(body) != string(quotedBody) || r.Header.Get("X-Platform-Execution-Authorization") != "exa_media_run" || r.Header.Get("Cookie") != "" {
				t.Error("media dispatch changed approved authority or bytes")
			}
			w.WriteHeader(503)
		case strings.HasPrefix(r.URL.Path, "/internal/relay/media-requests/"):
			recoveries++
			w.WriteHeader(404)
		default:
			t.Errorf("unexpected request before dispatch: %s", r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	defer server.Close()
	env := map[string]string{"STUDIO_PLATFORM_ORIGIN": server.URL, "STUDIO_PLATFORM_PRODUCT_KEY": "fixture.secret", "STUDIO_PUBLIC_ORIGIN": "https://studio.example.test", "STUDIO_PLATFORM_SESSION_HOSTS": "studio.example.test", "STUDIO_PLATFORM_ALLOWED_ORIGINS": "https://studio.example.test"}
	gateway, err := platformintegration.NewGatewayFromEnv(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	s.studioPlatform = gateway
	if err := db.Create(&model.CanvasProject{ID: "media-canvas", UserID: "user", PayloadJSON: `{"nodes":[],"connections":[]}`}).Error; err != nil {
		t.Fatal(err)
	}
	session := platformclient.SessionContext{Host: "studio.example.test", Origin: "https://studio.example.test", Cookie: "session=fixture"}
	req := CloudAgentRequest{Prompt: "draw", CanvasID: "media-canvas", Model: "pi-model", ChannelID: StudioPlatformChannelID, ChannelModelKey: "pi-model", IdempotencyKey: "pi-media", PermissionMode: "request_approval"}
	req.Budget.MaxSteps = 4
	req.ContextScope = []string{"canvas"}
	view, _, err := s.CreateStudioCloudAgentRun(context.Background(), "user", session, req, "")
	if err != nil {
		t.Fatal(err)
	}
	run, _ := s.repo.CloudAgent("user", view.ID)
	state, _ := cloudAgentDecode(run)
	state.Calls = []cloudAgentCall{agentMediaCall(cloudAgentMediaArgs{Mode: "image", Prompt: "a green square", ChannelID: StudioPlatformChannelID, ChannelModelKey: "image-model", Size: "1:1", NodeID: "image-node", Title: "草稿"})}
	if err := s.repo.MutateCloudAgent("user", run.ID, run.Revision, func(current *model.CloudAgentExecution, _ *repository.Repository) error {
		return cloudAgentSave(current, &state)
	}); err != nil {
		t.Fatal(err)
	}
	run, _ = s.repo.CloudAgent("user", view.ID)
	if err := s.advanceCloudAgentTool(run, &state); err != nil {
		t.Fatal(err)
	}
	run, _ = s.repo.CloudAgent("user", view.ID)
	state, _ = cloudAgentDecode(run)
	if run.Status != "waiting_approval" || state.Approval == nil {
		t.Fatalf("media draft did not reach native Pi approval: status=%s events=%+v", run.Status, state.Events)
	}
	approval := state.Approval.ID
	if err := s.DecideStudioAgentMedia("user", run.ID, approval, "approve", "", "", nil); err == nil {
		t.Fatal("unquoted media approved")
	}
	quote, _, err := s.QuoteStudioAgentMedia(context.Background(), "user", session, run.ID, approval)
	if err != nil {
		t.Fatal(err)
	}
	if quote.WalletMaxPoints != "0.123456789" || quote.PriceVersion != "price-1" {
		t.Fatal("decimal quote changed")
	}
	if _, _, err := s.QuoteStudioAgentMedia(context.Background(), "other", session, run.ID, approval); err == nil {
		t.Fatal("other account quoted run")
	}
	if err := s.DecideStudioAgentMedia("user", run.ID, approval, "approve", "", "old-quote", nil); err == nil {
		t.Fatal("stale quote approved")
	}
	if scenario != "approve" {
		run, _ = s.repo.CloudAgent("user", view.ID)
		state, _ = cloudAgentDecode(run)
		switch scenario {
		case "expired":
			state.Approval.StudioQuote.ExpiresAt = time.Now().Add(-time.Minute)
			if err := s.repo.MutateCloudAgent("user", run.ID, run.Revision, func(current *model.CloudAgentExecution, _ *repository.Repository) error {
				return cloudAgentSave(current, &state)
			}); err != nil {
				t.Fatal(err)
			}
		case "changed_canvas":
			var canvas model.CanvasProject
			db.First(&canvas, "id = ?", "media-canvas")
			var doc map[string]any
			_ = json.Unmarshal([]byte(canvas.PayloadJSON), &doc)
			nodes := creationMaps(doc["nodes"])
			nodes[0]["locked"] = true
			doc["nodes"] = nodes
			raw, _ := json.Marshal(doc)
			db.Model(&canvas).Update("payload_json", string(raw))
		case "stopped":
			db.Model(&model.CloudAgentExecution{}).Where("id = ?", run.ID).Update("status", "cancelled")
		case "reject":
			if err := s.DecideStudioAgentMedia("user", run.ID, approval, "reject", "", "", nil); err != nil {
				t.Fatal(err)
			}
		}
		if err := s.DecideStudioAgentMedia("user", run.ID, approval, "approve", "", quote.ID, nil); err == nil {
			t.Fatal("invalidated approval submitted media")
		}
		var steps int64
		db.Model(&model.StudioExecution{}).Where("command_id LIKE ?", "agent-media:%").Count(&steps)
		if steps != 0 || submits != 0 {
			t.Fatal("invalid approval created billed work")
		}
		return
	}
	// Hold the native waiter so this fixture can inspect the atomic queued checkpoint.
	s.claimCloudAgentApprovalFollowUp(cloudAgentApprovalFollowUpKey(run.ID, approval))
	defer s.releaseCloudAgentApprovalFollowUp(cloudAgentApprovalFollowUpKey(run.ID, approval))
	if err := s.DecideStudioAgentMedia("user", run.ID, approval, "approve", "", quote.ID, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.DecideStudioAgentMedia("user", run.ID, approval, "approve", "", quote.ID, nil); err != nil {
		t.Fatal(err)
	}
	run, _ = s.repo.CloudAgent("user", view.ID)
	state, _ = cloudAgentDecode(run)
	if state.MediaTaskID == "" || len(state.TaskIDs) != 2 {
		t.Fatal("approval did not atomically queue exactly one media step")
	}
	execution, err := s.repo.StudioExecution("user", state.MediaTaskID)
	if err != nil {
		t.Fatal(err)
	}
	var frozen studioExecutionEnvelope
	if err := s.decryptStudioValue(execution.EnvelopeEncrypted, &frozen); err != nil {
		t.Fatal(err)
	}
	grant, _ := s.decryptSettingSecret(execution.AuthorizationEncrypted)
	if grant != "exa_media_run" || !execution.Released || frozen.Consumption.WalletMaxPoints != quote.WalletMaxPoints || !strings.Contains(string(frozen.Body), "a green square") {
		t.Fatal("frozen request or shared authority changed")
	}
	if quotes != 1 || issues != 1 {
		t.Fatalf("unexpected quote/grant count %d/%d", quotes, issues)
	}
	for i := 0; i < 3; i++ {
		db.Model(&model.Task{}).Where("id = ?", state.MediaTaskID).Update("next_poll_at", time.Now().Add(-time.Second))
		if err := s.ProcessNextTask(); err != nil {
			t.Fatal(err)
		}
	}
	if submits != 1 || recoveries != 2 {
		t.Fatalf("ambiguous media resubmitted: submits=%d recovery=%d", submits, recoveries)
	}
	publicRun, err := s.CloudAgentRun("user", run.ID)
	if err != nil {
		t.Fatal(err)
	}
	publicJSON, _ := json.Marshal(publicRun)
	if strings.Contains(string(publicJSON), "studioPreparedEncrypted") || strings.Contains(string(publicJSON), "exa_media_run") {
		t.Fatal("private quote leaked to browser")
	}
	// Once the original output has been adopted, native Pi writes the fixed
	// Resource back without requiring a second local Asset.
	if err := db.Create(&model.Resource{ID: "generated-fixed", UserID: "user", Provider: "platform", Kind: "image", Status: model.ResourceStatusReady, MimeType: "image/png"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.StudioMaterialReference{ResourceID: "generated-fixed", UserID: "user", AssetID: "output-asset", VersionID: "v1", FileID: "f1", BindingID: "b1"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.Task{}).Where("id = ?", state.MediaTaskID).Updates(map[string]any{"status": model.TaskStatusSucceeded, "result_json": `{"images":[{"dataUrl":"/api/resources/generated-fixed/file","storageKey":"resource:generated-fixed","mimeType":"image/png"}]}`}).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.settleCloudAgentMedia("user", run.ID); err != nil {
		t.Fatal(err)
	}
	var canvas model.CanvasProject
	db.First(&canvas, "id = ?", "media-canvas")
	if !strings.Contains(canvas.PayloadJSON, "resource:generated-fixed") || !strings.Contains(canvas.PayloadJSON, `"status":"success"`) {
		t.Fatal("fixed media result not written to canvas")
	}
	var assets int64
	db.Model(&model.Asset{}).Count(&assets)
	if assets != 0 {
		t.Fatal("Pi created a duplicate local material")
	}
	var bills int64
	db.Model(&model.BillingOrder{}).Count(&bills)
	if bills != 0 {
		t.Fatal("created a local bill")
	}
}
