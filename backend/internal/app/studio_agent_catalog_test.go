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

func TestStudioAgentCatalogRequiresMaterialFromPublishedSchema(t *testing.T) {
	s, db, _, _ := creationTestService(t)
	if err := db.AutoMigrate(&model.StudioExecution{}, &model.StudioMaterialReference{}); err != nil {
		t.Fatal(err)
	}
	for _, row := range []any{
		&model.CanvasProject{ID: "canvas", UserID: "user", PayloadJSON: `{"nodes":[{"id":"image","type":"image","metadata":{"storageKey":"resource:ref"}}]}`},
		&model.Resource{ID: "ref", UserID: "user", Provider: "platform", Kind: "image", Status: model.ResourceStatusReady, MimeType: "image/png", Size: 12},
		&model.StudioMaterialReference{ResourceID: "ref", UserID: "user", AssetID: "asset", VersionID: "version", FileID: "file"},
	} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, protocol := range []string{"sora.video", "newapi.video", "modelark.video"} {
		t.Run(protocol, func(t *testing.T) {
			item := studioNativeFixtureModel(t, protocol)
			var profile platformintegration.MediaProfile
			if err := json.Unmarshal(item.MediaGenerationProfile, &profile); err != nil {
				t.Fatal(err)
			}
			base := profile.Variants[0]
			requiredMaterial := json.RawMessage(`{"type":"object","anyOf":[
				{"required":["metadata"],"properties":{"metadata":{"required":["images"],"properties":{"images":{"minItems":1}}}}},
				{"required":["metadata"],"properties":{"metadata":{"required":["videos"],"properties":{"videos":{"minItems":1}}}}}
			]}`)
			slots := []platformintegration.NativeReferenceSlot{
				{ID: "image", Role: "reference_image", Pointer: "/metadata/images", Transport: "url-array"},
				{ID: "video", Role: "reference_video", Pointer: "/metadata/videos", Transport: "url-array"},
			}
			if protocol == "modelark.video" {
				requiredMaterial = json.RawMessage(`{"type":"object","properties":{"content":{"allOf":[{"contains":{"required":["role"],"properties":{"role":{"enum":["reference_image","reference_video"]}}},"minContains":1}]}}}`)
				for i := range slots {
					slots[i].Pointer, slots[i].Transport = "/content", "content-array"
				}
			}
			zero, three := int64(0), int64(3)
			for i := range slots {
				slots[i].MinItems, slots[i].MaxItems = &zero, &three
			}
			profile.Variants = nil
			for _, id := range []string{"optional", "common", "only-supply", "optional-supply", "user-parameter"} {
				v := base
				n := *base.NativeRequest
				v.ID, n.ID, n.References = id, id, slots
				v.Bindings = map[string]string{"/resolution": "/resolution"}
				v.Schema = json.RawMessage(`{"type":"object","properties":{"/resolution":{"enum":["720p","1080p"]}}}`)
				n.Schema = json.RawMessage(`{"type":"object"}`)
				switch id {
				case "common":
					n.Schema = requiredMaterial
				case "only-supply", "optional-supply":
					options := []platformintegration.NativeSupplyAlternative{{Schema: requiredMaterial, References: slots}}
					if id == "optional-supply" {
						options = append(options, platformintegration.NativeSupplyAlternative{Schema: n.Schema, References: slots})
					}
					n.SupplyAlternatives, _ = json.Marshal(options)
				case "user-parameter":
					v.Defaults = map[string]any{}
					v.Schema = json.RawMessage(`{"type":"object","required":["/resolution"],"properties":{"/resolution":{"enum":["720p","1080p"]}}}`)
					n.Schema = json.RawMessage(`{"type":"object","required":["resolution"],"properties":{"resolution":{"enum":["720p","1080p"]}}}`)
				}
				v.NativeRequest = &n
				profile.Variants = append(profile.Variants, v)
			}
			item.MediaGenerationProfile, _ = json.Marshal(profile)
			frozen, err := s.encryptStudioValue(studioExecutionEnvelope{Catalog: []platformclient.Model{item}})
			if err != nil {
				t.Fatal(err)
			}
			if err := db.Create(&model.StudioExecution{TaskID: protocol, CommandID: protocol, UserID: "user", EnvelopeEncrypted: frozen}).Error; err != nil {
				t.Fatal(err)
			}
			state := &cloudAgentRuntime{RuntimeRunID: protocol, Request: CloudAgentRequest{CanvasID: "canvas"}}
			for _, tc := range []struct {
				args string
				want string
			}{
				{`{}`, "optional,optional-supply,user-parameter"},
				{`{"referenceNodeIds":["image"]}`, "optional,common,only-supply,optional-supply,user-parameter"},
				{`{"channelModelKey":"image-model","nativeVariantId":"common"}`, ""},
				{`{"channelModelKey":"image-model","nativeVariantId":"user-parameter"}`, "user-parameter"},
			} {
				call := cloudAgentCall{}
				call.Function.Name, call.Function.Arguments = "model_list", tc.args
				result, err := s.studioAgentModelList("user", state, call)
				if err != nil {
					t.Fatal(err)
				}
				page := result.(map[string]any)
				var ids []string
				for _, row := range page["models"].([]map[string]any) {
					ids = append(ids, row["selection"].(map[string]any)["nativeVariantId"].(string))
					if row["detailsLoaded"] == true && row["requiresParameters"] != true {
						t.Fatal("exact user-parameter candidate must identify the unfilled settings")
					}
				}
				if strings.Join(ids, ",") != tc.want || page["totalMatches"] != len(ids) {
					t.Errorf("model_list(%s) returned %v, want %s", tc.args, ids, tc.want)
				}
			}
		})
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
