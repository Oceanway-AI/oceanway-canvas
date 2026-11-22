package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"yingce/backend/internal/catalogtest"
	"yingce/backend/internal/model"
	"yingce/backend/internal/platformintegration"
	"yingce/backend/internal/repository"
)

const studioAgentMediaFixtureCatalog = `{"success":true,"data":{"items":[{"model_name":"pi-model","model_type":"text","invocation_status":"published_text","supported_protocols":["openai-chat"],"price":{"status":"available","version":"p1"}},{"model_name":"image-model","model_type":"image","invocation_status":"available","price":{"status":"available","version":"price-1"},"media_generation_profile":{"default_variant":"generate","variants":[{"id":"generate","protocol":"openai-images","operation":"generate","method":"POST","path":"/images/generations","defaults":{"n":1,"size":"1:1"},"specifications_schema":{"type":"object","additionalProperties":false,"properties":{"n":{"type":"integer","enum":[1]},"size":{"type":"string","enum":["1:1"]}},"required":["n","size"]},"parameter_bindings":{"n":"n","size":"size"}}]}}]}}`

func TestStudioPiMediaApprovalFreezesQuoteAndSharesAuthority(t *testing.T) {
	for _, scenario := range []string{"approve", "image_parameters", "video_const", "video_const_default", "video_const_invalid", "expired", "changed_canvas", "reject", "stopped", "missing_preview_capability", "missing_preview_connection", "missing_prepared_capability", "missing_prepared_connection", "missing_prepared_wallet_max", "changed_capability", "changed_connection", "missing_recovery_version", "missing_queued_capability", "missing_queued_connection", "missing_queued_wallet_max", "submitted_legacy"} {
		t.Run(scenario, func(t *testing.T) { studioPiMediaApprovalScenario(t, scenario) })
	}
}

func TestStudioPiNativeMediaApprovalPreservesProtocolAndRequest(t *testing.T) {
	for _, protocol := range []string{"openai.image.generate", "openai.image.edit", "gemini.image", "gemini.chat.image", "sora.video", "newapi.video", "modelark.video"} {
		t.Run(protocol, func(t *testing.T) { studioPiMediaApprovalScenario(t, "native_"+protocol) })
	}
}

func TestStudioPiGenerationAction(t *testing.T) {
	for _, scenario := range []string{"v1_manual_first", "v1_approval_first", "v1_manual_lost", "v1_approval_lost"} {
		t.Run(scenario, func(t *testing.T) { studioPiMediaApprovalScenario(t, scenario) })
	}
}

func TestStudioPiAutomaticMedia(t *testing.T) {
	for _, scenario := range []string{"v1_auto_image", "v1_auto_video", "auto_legacy", "auto_switch", "auto_read_only", "auto_pending"} {
		t.Run(scenario, func(t *testing.T) { studioPiMediaApprovalScenario(t, scenario) })
	}
}

func TestStudioPiGenerationActionPostgres(t *testing.T) {
	if dsn := os.Getenv("STUDIO_TEST_GENERATION_RESUME_DB"); dsn != "" {
		db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		if err != nil {
			t.Fatal(err)
		}
		sql, err := db.DB()
		if err != nil {
			t.Fatal(err)
		}
		defer sql.Close()
		gateway, err := platformintegration.NewGatewayFromEnv(os.Getenv)
		if err != nil {
			t.Fatal(err)
		}
		resumed := &Service{repo: repository.New(db), dataDir: os.Getenv("STUDIO_TEST_GENERATION_DATA"), studioPlatform: gateway}
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		task, err := resumed.waitStudioAgentGeneration(ctx, "user", os.Getenv("STUDIO_TEST_GENERATION_RUN"), os.Getenv("STUDIO_TEST_GENERATION_TASK"))
		if err != nil || task == nil || task.Status != model.TaskStatusFailed {
			t.Fatalf("process failed to resume original operation %v", err)
		}
		return
	}
	for _, scenario := range []string{"v1_manual_race_lost", "v1_approval_race_lost"} {
		t.Run(scenario, func(t *testing.T) { studioPiMediaApprovalScenario(t, scenario, true) })
	}
}

func TestStudioPiMediaPreviewDoesNotAuthorize(t *testing.T) {
	studioPiMediaApprovalScenario(t, "preview_only")
}

func TestStudioPiMediaRetainsApprovedReferenceInput(t *testing.T) {
	studioPiMediaApprovalScenario(t, "reference_input")
}

func studioPiMediaApprovalScenario(t *testing.T, scenario string, usePostgres ...bool) {
	var s *Service
	var db *gorm.DB
	if len(usePostgres) > 0 && usePostgres[0] {
		_, db = newProjectNameConflictPostgresService(t)
		s, db, _, _ = creationTestServiceOnDatabase(t, db)
	} else {
		s, db, _, _ = creationTestService(t)
	}
	if err := db.AutoMigrate(&model.StudioExecution{}, &model.StudioMaterialReference{}, &model.StudioMediaAction{}, &model.StudioMediaActionAlias{}); err != nil {
		t.Fatal(err)
	}
	s.legacyCloudAgentRootTask = false
	s.activeCancels = make(map[string]context.CancelFunc)
	quotes, issues := 0, 0
	submits, recoveries := 0, 0
	currentCapability, currentConnection := strings.Repeat("a", 64), int64(7)
	catalog := studioAgentMediaFixtureCatalog
	v1 := strings.HasPrefix(scenario, "v1_")
	postedID := ""
	postedLimit := ""
	runRef := ""
	race := strings.Contains(scenario, "race")
	entered := make(chan struct{}, 1)
	release := make(chan struct{}, 1)
	var complete atomic.Bool
	if v1 {
		catalog = strings.Replace(catalog, `"id":"generate","protocol"`, `"id":"generate","media_generation_v1":true,"protocol"`, 1)
	}
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
	if strings.HasPrefix(scenario, "video_const") || scenario == "v1_auto_video" {
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
	if scenario == "reference_input" {
		catalog = strings.Replace(catalog, `"parameter_bindings":`, `"native_reference_inputs":[{"field":"images","transport":"image-reference-array","roles":["reference_image"],"max_items":1}],"parameter_bindings":`, 1)
	}
	var quotedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case catalogtest.Route(r.URL.Path):
			catalogtest.Write(t, w, r, []byte(catalog))
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
		case v1 && strings.HasPrefix(r.URL.Path, "/internal/products/v1/media-generations"):

			if r.Method == "POST" {
				submits++
				var in platformclient.MediaCommand
				_ = json.NewDecoder(r.Body).Decode(&in)
				if in.RequestID == "manual-next" {
					if r.Header.Get("X-Platform-Execution-Authorization") != "" || r.Header.Get("Cookie") == "" || in.ProductTaskID == runRef {
						t.Error("fresh manual action inherited old approval grant")
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"request_id": in.RequestID, "task_id": "fresh-manual", "revision": 1, "execution_state": "accepted", "funds_state": "reserved", "outputs": []any{}})
					return
				}
				if r.Header.Get("X-Platform-Execution-Authorization") != "exa_media_run" {
					t.Error("winner lost original run authority")
				}
				postedID = in.RequestID
				runRef = in.ProductTaskID
				if in.Consumption != nil {
					postedLimit = in.Consumption.WalletMaxPoints
				} else {
					t.Error("missing media consumption contract")
				}
				if race {
					entered <- struct{}{}
					select {
					case <-release:
					case <-time.After(8 * time.Second):
						t.Error("winning POST was not released")
					}
				}
				if strings.HasSuffix(scenario, "lost") {
					w.WriteHeader(503)
					return
				}
			} else {
				recoveries++
				if r.Header.Get("X-Platform-Execution-Authorization") != "exa_media_run" {
					t.Error("read lost original run authority")
				}
				if !strings.HasSuffix(r.URL.Path, "/"+postedID) {
					t.Error("changed recovery id")
				}
			}
			if complete.Load() {
				_ = json.NewEncoder(w).Encode(map[string]any{"request_id": postedID, "task_id": "original-pi", "revision": 2, "execution_state": "failed", "funds_state": "released", "outputs": []any{}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"request_id": postedID, "task_id": "original-pi", "revision": 1, "execution_state": "running", "funds_state": "reserved", "outputs": []any{}, "retry_after_ms": 500})
		case r.URL.Path == submitPath:
			submits++
			if nativeProtocol != "" && kind == "image" && r.Header.Get("Prefer") != "respond-async" {
				t.Error("lost async mode")
			}
			body, _ := io.ReadAll(r.Body)
			raw, _ := base64.RawURLEncoding.DecodeString(r.Header.Get("X-Platform-Media-Consumption"))
			var constraints platformclient.MediaConsumptionConstraints
			if json.Unmarshal(raw, &constraints) != nil || constraints.ExpectedCapabilityRevision != strings.Repeat("a", 64) || constraints.ExpectedConnectionRevision != 7 || constraints.ExpectedPriceVersion != "price-1" || constraints.WalletMaxPoints != "" {
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
			if scenario == "changed_capability" || scenario == "changed_connection" || scenario == "missing_recovery_version" {
				receipt := platformclient.MediaRecovery{Version: "oceanway.media-request.v1", RequestID: strings.TrimPrefix(r.URL.Path, "/internal/relay/media-requests/"), Status: "FAILURE", NoExecution: true}
				if scenario == "missing_recovery_version" {
					receipt.Version = ""
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": receipt})
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
	if scenario == "reference_input" {
		for _, row := range []any{
			&model.Resource{ID: "agent-input", UserID: "user", Provider: "platform", Kind: "image", MimeType: "image/png", Size: 12, Status: model.ResourceStatusReady},
			&model.StudioMaterialReference{ResourceID: "agent-input", UserID: "user", AssetID: "input-asset", VersionID: "input-version", FileID: "input-file", BindingID: "input-binding"},
		} {
			if err := db.Create(row).Error; err != nil {
				t.Fatal(err)
			}
		}
		if err := db.Model(&model.CanvasProject{}).Where("id = ?", "media-canvas").Update("payload_json", `{"nodes":[{"id":"input-node","type":"image","width":320,"height":180,"position":{"x":0,"y":0},"metadata":{"storageKey":"resource:agent-input","content":"/api/resources/agent-input/file"}}],"connections":[]}`).Error; err != nil {
			t.Fatal(err)
		}
	}
	session := platformclient.SessionContext{Host: "studio.example.test", Origin: "https://studio.example.test", Cookie: "session=fixture"}
	req := CloudAgentRequest{Prompt: "draw", CanvasID: "media-canvas", Model: "pi-model", ChannelID: StudioPlatformChannelID, ChannelModelKey: "pi-model", IdempotencyKey: "pi-media", PermissionMode: "request_approval"}
	automatic := strings.Contains(scenario, "auto") && scenario != "auto_pending" && scenario != "auto_read_only"
	if automatic && scenario != "auto_switch" {
		req.PermissionMode = "auto"
	}
	if scenario == "auto_read_only" {
		req.PermissionMode = "read_only"
	}
	req.Budget.MaxSteps = 4
	req.ContextScope = []string{"canvas"}
	view, _, err := s.CreateStudioCloudAgentRun(context.Background(), "user", session, req, "")
	if err != nil {
		t.Fatal(err)
	}
	run, _ := s.repo.CloudAgent("user", view.ID)
	if scenario == "auto_switch" {
		if s.SetStudioAgentMode("other", run.ID, "request_approval", "auto") == nil {
			t.Fatal("other user changed mode")
		}
		if err := s.SetStudioAgentMode("user", run.ID, "request_approval", "auto"); err != nil {
			t.Fatal(err)
		}
		run, _ = s.repo.CloudAgent("user", view.ID)
	}
	state, _ := cloudAgentDecode(run)
	args := cloudAgentMediaArgs{Mode: "image", Prompt: "a green square", ChannelID: StudioPlatformChannelID, ChannelModelKey: "image-model", Size: "1:1", NodeID: "image-node", Title: "草稿"}
	if scenario == "reference_input" {
		args.ReferenceNodeIDs = []string{"input-node"}
	}
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
		if kind == "image" {
			// This native image fixture publishes no size field. Explicit public
			// aliases now must map to the published contract instead of vanishing.
			args.Size = ""
		}
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
	if scenario == "auto_read_only" {
		if state.MediaTaskID != "" || state.Approval != nil || submits != 0 || s.SetStudioAgentMode("user", run.ID, "request_approval", "auto") == nil {
			t.Fatal("read-only run acquired media authority")
		}
		return
	}
	if automatic {
		if run.Status != "running" || state.Approval != nil || state.MediaTaskID == "" || quotes != 0 || issues != 1 || len(state.TaskIDs) != 2 {
			t.Fatalf("automatic generation stalled: status=%s task=%s approvals=%v previews=%d grants=%d", run.Status, state.MediaTaskID, state.Approval, quotes, issues)
		}
		s.approvedMediaClosed = true
		for i := 0; i < 2; i++ {
			got, err := s.executeCloudAgentRuntimeTool(context.Background(), "user", run.ID, agentMediaCall(args))
			if err != nil || got.(map[string]any)["content"] != cloudAgentPiAutomaticMediaPauseText {
				t.Fatalf("retry failed to rejoin original task: %v %v", got, err)
			}
		}
		if v1 {
			if submits != 1 || postedLimit != "" || runRef != run.ID {
				t.Fatal("automatic generation changed grant or submitted twice")
			}
		} else {
			ex, err := s.repo.StudioExecution("user", state.MediaTaskID)
			var e studioExecutionEnvelope
			if err != nil || s.decryptStudioValue(ex.EnvelopeEncrypted, &e) != nil || !e.ActualUsageBilling || !e.validMediaConsumption() || e.Consumption.WalletMaxPoints != "" {
				t.Fatal("automatic relay did not freeze actual-usage terms")
			}
		}
		return
	}
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
	if scenario == "preview_only" {
		originalRevision := run.Revision
		for i := 0; i < 2; i++ {
			preview, _, err := s.QuoteStudioAgentMedia(context.Background(), "user", session, run.ID, approval, false)
			if err != nil {
				t.Fatal(err)
			}
			if preview.ID != "" || preview.WalletMaxPoints != "" || preview.EstimatedPoints != "0.123456789" {
				t.Fatalf("unconfirmed preview became an executable quote: %+v", preview)
			}
			reloaded, _ := s.repo.CloudAgent("user", view.ID)
			restored, _ := cloudAgentDecode(reloaded)
			if reloaded.Revision != originalRevision || restored.Approval.StudioQuote != nil || restored.Approval.StudioPreparedEncrypted != "" {
				t.Fatal("preview/cancel/reload persisted approval authority")
			}
			if err := s.DecideStudioAgentMedia("user", run.ID, approval, "approve", "", preview.ID, nil); err == nil {
				t.Fatal("preview allowed approval")
			}
		}
		confirmed, _, err := s.QuoteStudioAgentMedia(context.Background(), "user", session, run.ID, approval, true)
		if err != nil || confirmed.ID == "" {
			t.Fatalf("confirmed maximum not frozen: %+v %v", confirmed, err)
		}
		if _, _, err := s.QuoteStudioAgentMedia(context.Background(), "user", session, run.ID, approval, false); err != nil {
			t.Fatal(err)
		}
		reloaded, _ := s.repo.CloudAgent("user", view.ID)
		restored, _ := cloudAgentDecode(reloaded)
		if restored.Approval.StudioQuote.ID != confirmed.ID || restored.Approval.StudioQuote.WalletMaxPoints != "" || submits != 0 || issues != 1 {
			t.Fatal("preview replaced confirmed scope or submitted work")
		}
		return
	}
	if err := s.DecideStudioAgentMedia("user", run.ID, approval, "approve", "", "", nil); err == nil {
		t.Fatal("unquoted media approved")
	}
	quote, _, err := s.QuoteStudioAgentMedia(context.Background(), "user", session, run.ID, approval, true)
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
	if quote.BillingMode != "actual_usage" || quote.WalletMaxPoints != "" || quote.EstimatedPoints != "0.123456789" || quote.PriceVersion != "price-1" {
		t.Fatal("decimal quote changed")
	}
	if scenario == "auto_pending" {
		s.approvedMediaClosed = true
		if err := s.DecideStudioAgentMedia("user", run.ID, approval, "approve", "", quote.ID, nil, true); err != nil {
			t.Fatal(err)
		}
		latest, err := s.CloudAgentRun("user", run.ID)
		if err != nil || latest.Status != "running" || latest.PermissionMode != "auto" {
			t.Fatalf("pending draft and mode did not advance together: %v %+v", err, latest)
		}
		return
	}
	if kind == "video" {
		var body map[string]any
		if err := json.Unmarshal(quotedBody, &body); err != nil || body["duration"] != float64(4) || body["ratio"] != "16:9" {
			t.Fatalf("Pi lost the selected native parameters: %s", quotedBody)
		}
		resolution, sent := body["resolution"]
		if nativeProtocol == "" && resolution != "720p" || nativeProtocol != "" && sent {
			t.Fatalf("Pi did not distinguish required resolution from optional default: %s", quotedBody)
		}
	}
	if scenario == "image_parameters" {
		var body map[string]any
		if err := json.Unmarshal(quotedBody, &body); err != nil || body["background"] != "transparent" {
			t.Fatalf("published imageParameters did not override the default in the native quote: %s", quotedBody)
		}
	}
	if _, _, err := s.QuoteStudioAgentMedia(context.Background(), "other", session, run.ID, approval, true); err == nil {
		t.Fatal("other account quoted run")
	}
	if err := s.DecideStudioAgentMedia("user", run.ID, approval, "approve", "", "old-quote", nil); err == nil {
		t.Fatal("stale quote approved")
	}
	if scenario == "expired" || scenario == "changed_canvas" || scenario == "reject" || scenario == "stopped" || strings.HasPrefix(scenario, "missing_prepared_") {
		run, _ = s.repo.CloudAgent("user", view.ID)
		state, _ = cloudAgentDecode(run)
		switch scenario {
		case "missing_prepared_capability", "missing_prepared_connection", "missing_prepared_wallet_max":
			var prepared studioAgentMediaPrepared
			if err := s.decryptStudioValue(state.Approval.StudioPreparedEncrypted, &prepared); err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "missing_prepared_capability":
				prepared.Envelope.Consumption.ExpectedCapabilityRevision = ""
			case "missing_prepared_connection":
				prepared.Envelope.Consumption.ExpectedConnectionRevision = 0
			case "missing_prepared_wallet_max":
				prepared.Envelope.ActualUsageBilling = false // Old capped record with a missing limit.
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
	if v1 {
		// Exercise the actual action while the live Pi continuation is held.
		s.approvedMediaClosed = true
		others := []model.StudioMediaAction{{UserID: "user", ProjectID: "media-canvas", NodeID: "other-node", RunID: "other-run", ApprovalID: "other-approval"}, {UserID: "user", ProjectID: "different-canvas", NodeID: "image-node", RunID: "different-project-run", ApprovalID: "different-approval"}}
		for _, other := range others {
			if err := s.repo.OpenStudioMediaAction(other); err != nil {
				t.Fatal(err)
			}
		}
		manual := func() *model.Task {
			req := CreateTaskRequest{CommandID: "manual-original", ProjectID: "media-canvas", Type: "canvas_image", Operation: "image", Prompt: "manual image", Model: "image-model", Platform: &StudioTaskOptions{ExpectedPriceVersion: "price-1", WalletMaxPoints: "0"}, Input: map[string]any{"mode": "image", "config": map[string]any{"model": "image-model", "channelId": StudioPlatformChannelID, "imageParameters": map[string]any{"count": 1, "size": "1:1"}}, "metadata": map[string]any{"nodeId": "batch-child", "sourceNodeId": "image-node", "studioPendingRunId": run.ID}}}
			task, _, err := s.StudioCreateTask(context.Background(), "user", session, req)
			if err != nil {
				t.Fatal(err)
			}
			return task
		}
		approve := func() {
			if err := s.DecideStudioAgentMedia("user", run.ID, approval, "approve", "", quote.ID, nil); err != nil {
				t.Fatal(err)
			}
		}
		var winner *model.Task
		manualFirst := strings.HasPrefix(scenario, "v1_manual")
		if race {
			done := make(chan struct{})
			go func() {
				defer close(done)
				if manualFirst {
					winner = manual()
				} else {
					approve()
				}
			}()
			select {
			case <-entered:
			case <-time.After(8 * time.Second):
				t.Fatal("winning action did not dispatch")
			}
			if manualFirst {
				approve()
			} else {
				winner = manual()
			}
			release <- struct{}{}
			<-done
		} else if manualFirst {
			winner = manual()
			approve()
		} else {
			approve()
			winner = manual()
		}
		approve()
		original, err := s.StudioTaskByCommand("user", "manual-original", "user")
		if err != nil || original.ID != winner.ID {
			t.Fatalf("manual alias lost %v %+v", err, original)
		}
		latest, _ := s.repo.CloudAgent("user", run.ID)
		checkpoint, _ := cloudAgentDecode(latest)
		if submits != 1 || runRef != run.ID || checkpoint.MediaTaskID != winner.ID || winner.PlatformGenerationRequest != postedID || latest.Status != "running" {
			t.Fatalf("posts=%d request=%s ref=%s winner=%s active=%s state=%s", submits, postedID, runRef, winner.ID, checkpoint.MediaTaskID, latest.Status)
		}
		if manualFirst && (postedID != "manual-original" || postedLimit != "0") || !manualFirst && postedLimit != "" {
			t.Fatalf("changed winner scope %s", postedLimit)
		}
		complete.Store(true)
		var got *model.Task
		var e error
		if race {
			driver, ok := db.Dialector.(*postgres.Dialector)
			if !ok {
				t.Fatal("missing isolated PostgreSQL database")
			}
			child := exec.Command(os.Args[0], "-test.run=^TestStudioPiGenerationActionPostgres$", "-test.timeout=20s")
			child.Env = append(os.Environ(), "STUDIO_TEST_GENERATION_RESUME_DB="+driver.Config.DSN, "STUDIO_TEST_GENERATION_DATA="+s.dataDir, "STUDIO_TEST_GENERATION_RUN="+run.ID, "STUDIO_TEST_GENERATION_TASK="+winner.ID)
			for key, value := range env {
				child.Env = append(child.Env, key+"="+value)
			}
			if output, err := child.CombinedOutput(); err != nil {
				t.Fatalf("independent Studio process: %s", output)
			}
			got, e = s.repo.TaskForUser("user", winner.ID)
		} else {
			got, e = s.waitStudioAgentGeneration(context.Background(), "user", run.ID, winner.ID)
		}
		if e != nil || got.Status != model.TaskStatusFailed || submits != 1 || recoveries != 1 {
			t.Fatalf("original waiter grant/ID recovery task=%v err=%v POST=%d GET=%d", got, e, submits, recoveries)
		}
		fresh := CreateTaskRequest{CommandID: "manual-next", ProjectID: "media-canvas", Type: "canvas_image", Operation: "image", Model: "image-model", Prompt: "new user generation", Platform: &StudioTaskOptions{WalletMaxPoints: "0", ExpectedPriceVersion: "price-1"}, Input: map[string]any{"mode": "image", "config": map[string]any{"model": "image-model", "channelId": StudioPlatformChannelID, "imageParameters": map[string]any{"count": 1, "size": "1:1"}}, "metadata": map[string]any{"nodeId": "image-node"}}}
		newTask, _, err := s.StudioCreateTask(context.Background(), "user", session, fresh)
		if err != nil || newTask == nil || newTask.ID == winner.ID || newTask.PlatformGenerationRequest != "manual-next" || submits != 2 {
			t.Fatalf("fresh action reused completed winner %v", err)
		}
		for _, other := range others {
			unchanged, e := s.repo.StudioMediaAction(other.UserID, other.RunID, other.ApprovalID)
			if e != nil || unchanged.Winner != "" || unchanged.TaskID != "" {
				t.Fatalf("unrelated approval changed %+v %v", unchanged, e)
			}
		}
		return
	}

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
	if scenario == "reference_input" {
		task, err := s.repo.Task(state.MediaTaskID)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := documentReferencedResourceIDs(task.InputJSON, map[string]struct{}{"agent-input": {}})["agent-input"]; !ok {
			t.Errorf("approved media Task lost reference after native approval: %s", task.InputJSON)
		}
		if strings.Contains(task.InputJSON, "material://") || strings.Contains(task.InputJSON, "exa_") {
			t.Fatal("public Task exposes native authority")
		}
		if !strings.Contains(string(frozen.Body), "material://input-asset/input-version") {
			t.Fatal("native request changed fixed version")
		}
	}
	grant, _ := s.decryptSettingSecret(execution.AuthorizationEncrypted)
	if grant != "exa_media_run" || !execution.Released || frozen.Consumption.WalletMaxPoints != quote.WalletMaxPoints || !strings.Contains(string(frozen.Body), "a green square") {
		t.Fatal("frozen request or shared authority changed")
	}
	if quotes != 1 || issues != 1 {
		t.Fatalf("unexpected quote/grant count %d/%d", quotes, issues)
	}
	if scenario == "changed_capability" || scenario == "missing_recovery_version" {
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
		} else if scenario == "missing_queued_wallet_max" {
			frozen.ActualUsageBilling = false
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
	if scenario == "missing_recovery_version" {
		task, err := s.repo.Task(state.MediaTaskID)
		if err != nil {
			t.Fatal(err)
		}
		if task.Status != model.TaskStatusRunning || quotes != 1 || issues != 1 || submits != 1 || recoveries != 2 {
			t.Fatalf("unversioned rejection must keep querying the original request without resubmission: status=%s quote/grant/submit/recovery=%d/%d/%d/%d", task.Status, quotes, issues, submits, recoveries)
		}
		return
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
	if db.Migrator().HasTable("billing_orders") {
		t.Fatal("created a local bill")
	}
}
