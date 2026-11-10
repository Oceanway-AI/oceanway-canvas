package app

import (
	"context"
	"encoding/base64"
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
	for _, scenario := range []string{"approve", "image_parameters", "video_const", "video_const_default", "video_const_invalid", "expired", "changed_canvas", "reject", "stopped", "missing_preview_capability", "missing_preview_connection", "missing_prepared_capability", "missing_prepared_connection", "changed_capability", "changed_connection", "missing_queued_capability", "missing_queued_connection", "submitted_legacy"} {
		t.Run(scenario, func(t *testing.T) { studioPiMediaApprovalScenario(t, scenario) })
	}
}

func TestStudioPiNativeMediaApprovalPreservesProtocolAndRequest(t *testing.T) {
	for _, protocol := range []string{"openai.image.generate", "openai.image.edit", "gemini.image", "gemini.chat.image", "sora.video", "newapi.video", "modelark.video"} {
		t.Run(protocol, func(t *testing.T) { studioPiMediaApprovalScenario(t, "native_"+protocol) })
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
	currentCapability, currentConnection := strings.Repeat("a", 64), int64(7)
	catalog := studioAgentMediaFixtureCatalog
	kind, submitPath := "image", "/internal/relay/images/generations"
	nativeProtocol := ""
	if strings.HasPrefix(scenario, "native_") {
		nativeProtocol = strings.TrimPrefix(scenario, "native_")
		m := studioNativeFixtureModel(t, nativeProtocol)
		selected, err := platformintegration.SelectModel(m, "")
		if err != nil {
			t.Fatal(err)
		}
		kind, submitPath = m.ModelType, "/internal/relay/"+string(selected.MediaRoute)
		if selected.MediaRoute == platformclient.GeminiMedia {
			submitPath = "/internal/relay/v1beta/models/image-model:generateContent"
		}
		var envelope struct {
			Data struct {
				Items []platformclient.Model `json:"items"`
			} `json:"data"`
		}
		_ = json.Unmarshal([]byte(catalog), &envelope)
		raw, _ := json.Marshal(map[string]any{"success": true, "data": map[string]any{"items": []platformclient.Model{envelope.Data.Items[0], m}}})
		catalog = string(raw)
	}
	if strings.HasPrefix(scenario, "video_const") {
		kind, submitPath = "video", "/internal/relay/doubao/api/v3/contents/generations/tasks"
		catalog = strings.Replace(catalog, `"model_type":"image"`, `"model_type":"video"`, 1)
		catalog = strings.Replace(catalog, `"protocol":"openai-images"`, `"protocol":"doubao-video"`, 1)
		catalog = strings.Replace(catalog, `"path":"/images/generations"`, `"path":"/doubao/api/v3/contents/generations/tasks"`, 1)
		catalog = strings.Replace(catalog, `"defaults":{"n":1,"size":"1:1"}`, `"defaults":{}`, 1)
		catalog = strings.Replace(catalog, `"properties":{"n":{"type":"integer","enum":[1]},"size":{"type":"string","enum":["1:1"]}},"required":["n","size"]`, `"properties":{"resolution":{"type":"string","const":"720p"},"duration":{"type":"integer","enum":[4,5]},"ratio":{"type":"string","enum":["16:9"]}},"required":["resolution","duration","ratio"]`, 1)
		catalog = strings.Replace(catalog, `"parameter_bindings":{"n":"n","size":"size"}`, `"parameter_bindings":{"resolution":"resolution","duration":"duration","ratio":"ratio"}`, 1)
	}
	if scenario == "image_parameters" {
		// Publish a parameter which has no dedicated generate_media argument.
		catalog = strings.ReplaceAll(catalog, `"size":"1:1"`, `"size":"1:1","background":"opaque"`)
		catalog = strings.ReplaceAll(catalog, `"size":{"type":"string"`, `"background":{"type":"string","enum":["opaque","transparent"]},"size":{"type":"string"`)
		catalog = strings.ReplaceAll(catalog, `"size":"size"`, `"size":"size","background":"background"`)
	}
	var quotedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/models"):
			_, _ = w.Write([]byte(catalog))
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
			if preview.Request.NativeProtocol != nativeProtocol {
				t.Error("preview lost native protocol")
			}
			if r.Header.Get("Origin") != "https://studio.example.test" || r.Header.Get("Cookie") == "" {
				t.Error("quote lost genuine browser authority")
			}
			quote := platformclient.ConsumptionPreview{CapabilityRevision: strings.Repeat("a", 64), ConnectionRevision: 7, Model: "image-model", ModelType: kind, PriceVersion: "price-1", CombinationSupported: true, Allowed: true, WalletPoints: "0.123456789"}
			if scenario == "missing_preview_capability" {
				quote.CapabilityRevision = ""
			}
			if scenario == "missing_preview_connection" {
				quote.ConnectionRevision = 0
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": quote})
		case r.URL.Path == submitPath:
			submits++
			if nativeProtocol != "" && kind == "image" && r.Header.Get("Prefer") != "respond-async" {
				t.Error("lost async mode")
			}
			body, _ := io.ReadAll(r.Body)
			raw, _ := base64.RawURLEncoding.DecodeString(r.Header.Get("X-Platform-Media-Consumption"))
			var constraints platformclient.MediaConsumptionConstraints
			if json.Unmarshal(raw, &constraints) != nil || constraints.ExpectedCapabilityRevision != strings.Repeat("a", 64) || constraints.ExpectedConnectionRevision != 7 || constraints.ExpectedPriceVersion != "price-1" || constraints.WalletMaxPoints != "0.123456789" {
				t.Error("Pi consumption header lost confirmed versions")
			}
			if string(body) != string(quotedBody) || r.Header.Get("X-Platform-Execution-Authorization") != "exa_media_run" || r.Header.Get("Cookie") != "" {
				t.Error("media dispatch changed approved authority or bytes")
			}
			if constraints.ExpectedCapabilityRevision != currentCapability || constraints.ExpectedConnectionRevision != currentConnection {
				w.WriteHeader(409)
			} else {
				w.WriteHeader(503)
			}
		case strings.HasPrefix(r.URL.Path, "/internal/relay/media-requests/"):
			recoveries++
			if scenario == "changed_capability" || scenario == "changed_connection" {
				_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": platformclient.MediaRecovery{RequestID: strings.TrimPrefix(r.URL.Path, "/internal/relay/media-requests/"), Status: "FAILURE", NoExecution: true}})
				return
			}
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
	args := cloudAgentMediaArgs{Mode: "image", Prompt: "a green square", ChannelID: StudioPlatformChannelID, ChannelModelKey: "image-model", Size: "1:1", NodeID: "image-node", Title: "草稿"}
	if kind == "video" {
		args.Mode, args.Size, args.Duration = "video", "16:9", 4
		if scenario == "video_const" {
			args.Quality = "720"
		}
		if scenario == "video_const_invalid" {
			args.Quality = "1080"
		}
	}
	if scenario == "image_parameters" {
		args.ImageParameters = `{"background":"transparent"}`
	}
	if nativeProtocol != "" {
		args.NativeVariantID, args.NativeParameters = "choice", `{"/seed":0,"/metadata":null}`
	}
	state.Calls = []cloudAgentCall{agentMediaCall(args)}
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
	if scenario == "video_const_invalid" {
		if run.Status != "failed" || quotes != 0 || submits != 0 || state.Approval != nil {
			t.Fatal("unsupported Agent resolution was replaced or quoted")
		}
		var canvas model.CanvasProject
		db.First(&canvas, "id = ?", "media-canvas")
		if strings.Contains(canvas.PayloadJSON, "image-node") {
			t.Fatal("unsupported resolution created a media node")
		}
		return
	}
	if run.Status != "waiting_approval" || state.Approval == nil {
		t.Fatalf("media draft did not reach native Pi approval: status=%s events=%+v", run.Status, state.Events)
	}
	approval := state.Approval.ID
	if err := s.DecideStudioAgentMedia("user", run.ID, approval, "approve", "", "", nil); err == nil {
		t.Fatal("unquoted media approved")
	}
	quote, _, err := s.QuoteStudioAgentMedia(context.Background(), "user", session, run.ID, approval)
	if strings.HasPrefix(scenario, "missing_preview_") {
		if appErr, ok := err.(*AppError); !ok || appErr.Status != 409 {
			t.Fatalf("incomplete preview not rejected: %v", err)
		}
		run, _ = s.repo.CloudAgent("user", view.ID)
		state, _ = cloudAgentDecode(run)
		if state.Approval.StudioQuote != nil || state.Approval.StudioPreparedEncrypted != "" || submits != 0 || quotes != 1 || issues != 1 {
			t.Fatal("incomplete preview created approval/execution")
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	if quote.WalletMaxPoints != "0.123456789" || quote.PriceVersion != "price-1" {
		t.Fatal("decimal quote changed")
	}
	if kind == "video" {
		var body map[string]any
		if err := json.Unmarshal(quotedBody, &body); err != nil || body["resolution"] != "720p" || body["duration"] != float64(4) || body["ratio"] != "16:9" {
			t.Fatalf("Pi lost const resolution or native parameters: %s", quotedBody)
		}
	}
	if scenario == "image_parameters" {
		var body map[string]any
		if err := json.Unmarshal(quotedBody, &body); err != nil || body["background"] != "transparent" {
			t.Fatalf("published imageParameters did not override the default in the native quote: %s", quotedBody)
		}
	}
	if _, _, err := s.QuoteStudioAgentMedia(context.Background(), "other", session, run.ID, approval); err == nil {
		t.Fatal("other account quoted run")
	}
	if err := s.DecideStudioAgentMedia("user", run.ID, approval, "approve", "", "old-quote", nil); err == nil {
		t.Fatal("stale quote approved")
	}
	if scenario == "expired" || scenario == "changed_canvas" || scenario == "reject" || scenario == "stopped" || strings.HasPrefix(scenario, "missing_prepared_") {
		run, _ = s.repo.CloudAgent("user", view.ID)
		state, _ = cloudAgentDecode(run)
		switch scenario {
		case "missing_prepared_capability", "missing_prepared_connection":
			var prepared studioAgentMediaPrepared
			if err := s.decryptStudioValue(state.Approval.StudioPreparedEncrypted, &prepared); err != nil {
				t.Fatal(err)
			}
			if scenario == "missing_prepared_capability" {
				prepared.Envelope.Consumption.ExpectedCapabilityRevision = ""
			} else {
				prepared.Envelope.Consumption.ExpectedConnectionRevision = 0
			}
			state.Approval.StudioPreparedEncrypted, err = s.encryptStudioValue(prepared)
			if err != nil {
				t.Fatal(err)
			}
			if err := s.repo.MutateCloudAgent("user", run.ID, run.Revision, func(current *model.CloudAgentExecution, _ *repository.Repository) error {
				return cloudAgentSave(current, &state)
			}); err != nil {
				t.Fatal(err)
			}
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
	if frozen.Consumption == nil || frozen.Consumption.ExpectedCapabilityRevision != strings.Repeat("a", 64) || frozen.Consumption.ExpectedConnectionRevision != 7 {
		t.Fatal("Pi encrypted envelope lost confirmed capability/connection")
	}
	grant, _ := s.decryptSettingSecret(execution.AuthorizationEncrypted)
	if grant != "exa_media_run" || !execution.Released || frozen.Consumption.WalletMaxPoints != quote.WalletMaxPoints || !strings.Contains(string(frozen.Body), "a green square") {
		t.Fatal("frozen request or shared authority changed")
	}
	if quotes != 1 || issues != 1 {
		t.Fatalf("unexpected quote/grant count %d/%d", quotes, issues)
	}
	if scenario == "changed_capability" {
		currentCapability = strings.Repeat("b", 64)
	}
	if scenario == "changed_connection" {
		currentConnection = 8
	}
	if strings.HasPrefix(scenario, "missing_queued_") || scenario == "submitted_legacy" {
		if scenario == "submitted_legacy" {
			if err := s.ProcessNextTask(); err != nil {
				t.Fatal(err)
			}
			frozen.Consumption = nil
		} else if scenario == "missing_queued_capability" {
			frozen.Consumption.ExpectedCapabilityRevision = ""
		} else {
			frozen.Consumption.ExpectedConnectionRevision = 0
		}
		encrypted, err := s.encryptStudioValue(frozen)
		if err != nil {
			t.Fatal(err)
		}
		if err := db.Model(&model.StudioExecution{}).Where("task_id = ?", state.MediaTaskID).Update("envelope_encrypted", encrypted).Error; err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 3; i++ {
		db.Model(&model.Task{}).Where("id = ?", state.MediaTaskID).Update("next_poll_at", time.Now().Add(-time.Second))
		if err := s.ProcessNextTask(); err != nil {
			t.Fatal(err)
		}
	}
	if strings.HasPrefix(scenario, "missing_queued_") || strings.HasPrefix(scenario, "changed_") || scenario == "submitted_legacy" {
		task, err := s.repo.Task(state.MediaTaskID)
		if err != nil {
			t.Fatal(err)
		}
		wantSubmits, wantRecovers := 1, 1
		if strings.HasPrefix(scenario, "missing_queued_") {
			wantSubmits, wantRecovers = 0, 0
		}
		if scenario == "submitted_legacy" {
			wantRecovers = 3
		} else if task.Status != model.TaskStatusFailed {
			t.Fatalf("invalid frozen media not failed: %s", task.Status)
		}
		if quotes != 1 || issues != 1 || submits != wantSubmits || recoveries != wantRecovers {
			t.Fatalf("invalid/legacy consumption retried/requoted: %d/%d/%d/%d", quotes, issues, submits, recoveries)
		}
		return
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
	mimeType := "image/png"
	result := `{"images":[{"dataUrl":"/api/resources/generated-fixed/file","storageKey":"resource:generated-fixed","mimeType":"image/png"}]}`
	if kind == "video" {
		mimeType = "video/mp4"
		result = `{"video":{"dataUrl":"/api/resources/generated-fixed/file","storageKey":"resource:generated-fixed","mimeType":"video/mp4"}}`
	}
	if err := db.Create(&model.Resource{ID: "generated-fixed", UserID: "user", Provider: "platform", Kind: kind, Status: model.ResourceStatusReady, MimeType: mimeType}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.StudioMaterialReference{ResourceID: "generated-fixed", UserID: "user", AssetID: "output-asset", VersionID: "v1", FileID: "f1", BindingID: "b1"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.Task{}).Where("id = ?", state.MediaTaskID).Updates(map[string]any{"status": model.TaskStatusSucceeded, "result_json": result}).Error; err != nil {
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
