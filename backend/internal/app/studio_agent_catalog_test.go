package app

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"yingce/backend/internal/model"
	"yingce/backend/internal/platformintegration"
)

func TestStudioAgentCatalogPagesFrozenLargeProfiles(t *testing.T) {
	s, db, _, _ := creationTestService(t)
	if err := db.AutoMigrate(&model.StudioExecution{}); err != nil {
		t.Fatal(err)
	}
	base := studioNativeFixtureModel(t, "modelark.video")
	var profile map[string]any
	_ = json.Unmarshal(base.MediaGenerationProfile, &profile)
	profile["description"] = strings.Repeat("published contract ", 10000)
	base.MediaGenerationProfile, _ = json.Marshal(profile)
	models := []platformclient.Model{}
	for i := 0; i < 30; i++ {
		item := base
		item.ModelName = fmt.Sprintf("video-%02d", i)
		models = append(models, item)
	}
	encrypted, err := s.encryptStudioValue(studioExecutionEnvelope{Catalog: models})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.StudioExecution{TaskID: "catalog-run", UserID: "user", EnvelopeEncrypted: encrypted}).Error; err != nil {
		t.Fatal(err)
	}
	state := &cloudAgentRuntime{RuntimeRunID: "catalog-run"}
	seen := map[string]bool{}
	offset := 0
	for {
		call := cloudAgentCall{}
		call.Function.Name = "model_list"
		call.Function.Arguments = fmt.Sprintf(`{"mode":"video","offset":%d}`, offset)
		result, err := s.studioAgentModelList("user", state, call)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(result)
		if len(raw) > studioAgentCatalogPageBytes || strings.Contains(string(raw), "nativeProfile") || strings.Contains(string(raw), "specificationsSchema") {
			t.Fatal("summary leaked full contracts or exceeded byte budget")
		}
		page := result.(map[string]any)
		for _, row := range page["models"].([]map[string]any) {
			name := row["name"].(string)
			if seen[name] {
				t.Fatal("paging duplicated a candidate")
			}
			seen[name] = true
		}
		next := page["nextOffset"].(int)
		if page["hasMore"] == false {
			break
		}
		if next <= offset {
			t.Fatal("paging made no progress")
		}
		offset = next
	}
	if len(seen) != len(models) {
		t.Fatalf("paged %d of %d models", len(seen), len(models))
	}
	call := cloudAgentCall{}
	call.Function.Name = "model_list"
	call.Function.Arguments = `{"mode":"video","channelModelKey":"video-01","nativeVariantId":"choice"}`
	result, err := s.studioAgentModelList("user", state, call)
	if err != nil {
		t.Fatal(err)
	}
	row := result.(map[string]any)["models"].([]map[string]any)[0]
	if row["detailsLoaded"] != true || row["specificationsSchema"] == nil || row["generationArguments"].(map[string]any)["nativeParameters"] == nil {
		t.Fatal("exact variant details lost the native parameter contract")
	}
}

func TestStudioAgentCatalogMatchesActualReferenceRoles(t *testing.T) {
	s, db, _, _ := creationTestService(t)
	if err := db.AutoMigrate(&model.StudioExecution{}, &model.StudioMaterialReference{}); err != nil {
		t.Fatal(err)
	}
	for _, row := range []any{
		&model.CanvasProject{ID: "agent-canvas", UserID: "user", PayloadJSON: `{"nodes":[{"id":"image","type":"image","metadata":{"storageKey":"resource:ref"}}]}`},
		&model.Resource{ID: "ref", UserID: "user", Provider: "platform", Kind: "image", Status: model.ResourceStatusReady, MimeType: "image/png", Size: 12, Width: 16, Height: 16},
		&model.StudioMaterialReference{ResourceID: "ref", UserID: "user", AssetID: "asset", VersionID: "version", FileID: "file"},
	} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	item := studioNativeFixtureModel(t, "modelark.video")
	var profile platformintegration.MediaProfile
	_ = json.Unmarshal(item.MediaGenerationProfile, &profile)
	base := profile.Variants[0]
	for _, role := range []string{"reference_image", "first_frame"} {
		variant := base
		variant.ID = role
		native := *base.NativeRequest
		one := int64(1)
		native.References = []platformintegration.NativeReferenceSlot{{ID: "ref", Pointer: "/content", Role: role, Transport: "content-array", MinItems: &one, MaxItems: &one, MIMETypes: []string{"image/png"}}}
		variant.NativeRequest = &native
		profile.Variants = append(profile.Variants, variant)
	}
	item.MediaGenerationProfile, _ = json.Marshal(profile)
	encrypted, err := s.encryptStudioValue(studioExecutionEnvelope{Catalog: []platformclient.Model{item}})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.StudioExecution{TaskID: "catalog-run", UserID: "user", EnvelopeEncrypted: encrypted}).Error; err != nil {
		t.Fatal(err)
	}
	state := &cloudAgentRuntime{RuntimeRunID: "catalog-run", Request: CloudAgentRequest{CanvasID: "agent-canvas"}}
	for _, tc := range []struct{ args, variant string }{
		{`{"mode":"video"}`, "choice"},
		{`{"mode":"video","referenceNodeIds":["image"]}`, "reference_image"},
		{`{"mode":"video","referenceNodeIds":["image"],"videoStartFrameNodeId":"image"}`, "first_frame"},
		{`{"mode":"video","referenceNodeIds":["missing"]}`, ""},
		{`{"mode":"video","videoStartFrameNodeId":"missing"}`, ""},
	} {
		call := cloudAgentCall{}
		call.Function.Name, call.Function.Arguments = "model_list", tc.args
		result, err := s.studioAgentModelList("user", state, call)
		if tc.variant == "" {
			if err == nil {
				t.Fatal("unavailable reference was accepted")
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		rows := result.(map[string]any)["models"].([]map[string]any)
		if len(rows) != 1 || rows[0]["selection"].(map[string]any)["nativeVariantId"] != tc.variant {
			t.Fatalf("wrong variant for %s", tc.args)
		}
	}
	call := cloudAgentCall{}
	call.Function.Name, call.Function.Arguments = "model_list", `{"mode":"video","referenceNodeIds":["image"]}`
	if _, err := cloudAgentReadToolCached(s.repo, "user", state, call, s); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.CanvasProject{}).Where("id = ?", "agent-canvas").Updates(map[string]any{"payload_json": `{"nodes":[]}`, "revision": 2}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := cloudAgentReadToolCached(s.repo, "user", state, call, s); err == nil {
		t.Fatal("model list reused references deleted in a newer canvas revision")
	}
}

func TestStudioFrameMetadataReachesNativeRoles(t *testing.T) {
	for _, same := range []bool{false, true} {
		end := "last"
		if same {
			end = "first"
		}
		input, err := studioDecodeInput(CreateTaskRequest{Input: map[string]any{
			"config":          map[string]any{"channelId": StudioPlatformChannelID},
			"referenceImages": []any{map[string]any{"id": "first"}, map[string]any{"id": "last"}},
			"metadata":        map[string]any{"videoStartFrameNodeId": "first", "videoEndFrameNodeId": end},
		}})
		if err != nil {
			t.Fatal(err)
		}
		roles := map[string]int{}
		for _, ref := range input.ReferenceImages {
			roles[ref.Role]++
		}
		if roles["first_frame"] != 1 || roles["last_frame"] != 1 {
			t.Fatal("explicit frame metadata lost before request construction")
		}
	}
}

func TestStudioAgentUniqueCatalogSelectionKeepsVariantAndFrameScope(t *testing.T) {
	for _, tc := range []struct {
		name             string
		hasMore          bool
		total            int
		variantsComplete bool
		frame            string
		want             bool
	}{
		{"unique", false, 1, true, "first", true},
		{"first page", true, 2, true, "first", false},
		{"last page", false, 2, true, "first", false},
		{"incomplete variants", false, 1, false, "first", false},
		{"different frame", false, 1, true, "other", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := cloudAgentRuntime{}
			state.event("run", "tool_completed", map[string]any{
				"toolName": "model_list", "arguments": `{"mode":"video","referenceNodeIds":["first"],"videoStartFrameNodeId":"first"}`,
				"result": map[string]any{
					"hasMore": tc.hasMore, "totalMatches": tc.total,
					"models": []map[string]any{{"capability": "video", "variantsComplete": tc.variantsComplete,
						"selection": map[string]any{"channelId": StudioPlatformChannelID, "channelModelKey": "video", "nativeVariantId": "first-frame"}}},
				},
			})
			args := cloudAgentMediaArgs{Mode: "video", ReferenceNodeIDs: []string{"first"}, VideoStartFrameNodeID: tc.frame}
			if got := cloudAgentApplyUniqueModelListSelection(&state, `{}`, &args); got != tc.want {
				t.Fatalf("unique selection = %v, want %v", got, tc.want)
			}
			if tc.want && args.NativeVariantID != "first-frame" {
				t.Fatal("unique selection lost the published variant")
			}
		})
	}
}
