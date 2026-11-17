package app

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"yingce/backend/internal/model"
	"yingce/backend/internal/platformintegration"
)

func TestStudioAgentPreflightUsesFrozenNativeContractWithoutCreatingTasks(t *testing.T) {
	s, db := agentMediaBatchFixture(t)
	run, state := agentMediaBatchRun(t, s, agentMediaBatchArgs("v1"), 4)
	if err := db.AutoMigrate(&model.StudioExecution{}); err != nil {
		t.Fatal(err)
	}
	canvas, _ := s.repo.CanvasProjectForUser("user", "agent-canvas")
	doc, _ := creationDocument(canvas.PayloadJSON)
	doc["connections"] = []any{}
	raw, _ := json.Marshal(doc)
	if err := db.Model(canvas).Update("payload_json", string(raw)).Error; err != nil {
		t.Fatal(err)
	}
	base := studioNativeFixtureModel(t, "modelark.video")
	encrypted, err := s.encryptStudioValue(studioExecutionEnvelope{Catalog: []platformclient.Model{base}})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.StudioExecution{UserID: "user", TaskID: run.ID, State: "agent", EnvelopeEncrypted: encrypted}).Error; err != nil {
		t.Fatal(err)
	}
	s.studioPlatform = &platformintegration.Gateway{}
	state.Request.ChannelID, state.Request.Budget.MaxCredits = StudioPlatformChannelID, 0
	call := cloudAgentCall{ID: "platform-preflight"}
	call.Function.Name = "canvas_preflight"
	call.Function.Arguments = `{"mode":"video","channelId":"oceanway-platform","channelModelKey":"image-model","nativeVariantId":"choice","nativeParameters":"{\"/duration\":8,\"/resolution\":\"1080p\"}","items":[{"nodeId":"v1"}]}`
	var before, after int64
	db.Model(&model.Task{}).Count(&before)
	result, err := cloudAgentHandleCanvasPreflight(s.repo, "user", &state, call, s)
	if err != nil {
		t.Fatal(err)
	}
	report := result.(map[string]any)
	if report["ok"] != true || report["quoteAvailable"] != false || report["fundingAuthority"] != "platform" || report["estimatedCredits"] != nil || report["remainingCredits"] != nil {
		t.Fatalf("invalid platform preflight: %#v", report)
	}
	if strings.Contains(stringValue(report["next"]), "generate_media_batch") {
		t.Fatal("preflight advertised unsupported batch execution")
	}
	db.Model(&model.Task{}).Count(&after)
	if after != before {
		t.Fatal("preflight created a task")
	}
}

func TestStudioAgentTaskFactsKeepPlatformSettlementPrecision(t *testing.T) {
	for _, tc := range []struct{ result, state, points string }{
		{`{"fundsState":"settled","settledPoints":"12.500000000000000001"}`, "settled", "12.500000000000000001"},
		{`{"fundsState":"reserved"}`, "reserved", ""},
		{`{"text":"completed","usage":{"output_tokens":10}}`, "unknown", ""},
	} {
		facts := cloudAgentTaskDiagnostic(nil, &model.Task{ID: "task", Provider: "platform", ResultJSON: tc.result})
		billing, ok := facts["platformBilling"].(map[string]any)
		if !ok || billing["fundsState"] != tc.state || stringValue(billing["settledPoints"]) != tc.points || facts["billing"] != nil {
			t.Fatalf("invalid platform facts: %#v", facts)
		}
		if tc.state == "unknown" && billing["stateAvailable"] != false {
			t.Fatal("missing platform settlement was treated as known zero")
		}
	}
}
