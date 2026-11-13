package app

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"yingce/backend/internal/model"
	"yingce/backend/internal/platformintegration"
)

func studioNativeFixtureModel(t *testing.T, protocol string) platformclient.Model {
	t.Helper()
	kind, path, envelope := "image", "/v1/images/generations", "prompt"
	switch protocol {
	case "openai.image.edit":
		path = "/v1/images/edits"
	case "gemini.image":
		path, envelope = "/v1beta/models/{model}:generateContent", "contents"
	case "gemini.chat.image":
		path, envelope = "/v1/chat/completions", "messages"
	case "modelark.video":
		kind, path, envelope = "video", "/api/v3/contents/generations/tasks", "content"
	case "sora.video":
		kind, path = "video", "/v1/videos"
	case "newapi.video":
		kind, path = "video", "/v1/video/generations"
	}
	props := map[string]any{"model": map[string]any{"const": "image-model"}, "seed": map[string]any{"type": "integer"}, "metadata": map[string]any{"type": []string{"object", "null"}}}
	if envelope == "prompt" {
		props[envelope] = map[string]any{"type": "string", "minLength": 1}
	} else {
		props[envelope] = map[string]any{"type": "array", "minItems": 1}
	}
	defaults := map[string]any{"seed": 7, "metadata": map[string]any{"tag": "test"}}
	if kind == "video" {
		props["duration"] = map[string]any{"type": "integer", "enum": []int{4, 8}}
		props["resolution"] = map[string]any{"type": "string", "enum": []string{"720p", "1080p"}}
		props["ratio"] = map[string]any{"type": "string", "enum": []string{"16:9", "9:16"}}
		props["generate_audio"] = map[string]any{"type": "boolean"}
		defaults["duration"], defaults["resolution"], defaults["ratio"], defaults["generate_audio"] = 4, "720p", "16:9", true
	} else {
		props["generationConfig"] = map[string]any{"type": "object", "properties": map[string]any{"responseModalities": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}, "imageConfig": map[string]any{"type": "object", "properties": map[string]any{"imageSize": map[string]any{"type": "string", "enum": []string{"1K", "2K"}}}, "required": []string{"imageSize"}}}, "required": []string{"responseModalities", "imageConfig"}}
		defaults["generationConfig"] = map[string]any{"responseModalities": []string{"IMAGE"}, "imageConfig": map[string]any{"imageSize": "1K"}}
	}
	full := map[string]any{"type": "object", "properties": props, "additionalProperties": false, "required": []string{envelope}}
	if kind == "video" {
		full["allOf"] = []any{map[string]any{"if": map[string]any{"properties": map[string]any{"duration": map[string]any{"const": 8}}}, "then": map[string]any{"properties": map[string]any{"resolution": map[string]any{"const": "1080p"}}}}}
	}
	fullRaw, _ := json.Marshal(full)
	native := &platformintegration.NativeRequestVariant{ID: "native", Protocol: protocol, Operation: "create", Method: "POST", Path: path, ContentTypes: []string{"application/json"}, Modes: []string{"sync", "async"}, MaxOutputs: 1, Defaults: defaults, Schema: fullRaw}
	flat, bindings, flatDefaults := map[string]any{}, map[string]string{}, map[string]any{}
	for k, v := range props {
		if k == "model" || k == envelope {
			continue
		}
		if k == "generationConfig" {
			flat["/generationConfig/responseModalities"] = map[string]any{"type": "array", "items": map[string]any{"type": "string"}}
			flat["/generationConfig/imageConfig/imageSize"] = map[string]any{"type": "string", "enum": []string{"1K", "2K"}}
			flatDefaults["/generationConfig/responseModalities"], flatDefaults["/generationConfig/imageConfig/imageSize"] = []string{"IMAGE"}, "1K"
		} else {
			flat["/"+k], flatDefaults["/"+k] = v, defaults[k]
		}
	}
	for k := range flat {
		bindings[k] = k
	}
	schema, _ := json.Marshal(map[string]any{"type": "object", "properties": flat, "additionalProperties": false})
	v := platformintegration.NativeEndpoint{ID: "choice", Method: "POST", GatewayProtocol: protocol, NativeRequest: native, Schema: schema, Defaults: flatDefaults, Bindings: bindings}
	raw, _ := json.Marshal(map[string]any{"default_variant": "choice", "native_request_variant": native, "gateway_protocol": protocol, "variants": []any{v}})
	return platformclient.Model{ModelName: "image-model", ModelType: kind, InvocationStatus: "available", Price: json.RawMessage(`{"status":"available","version":"price-1"}`), MediaGenerationProfile: raw}
}

func TestStudioPublishedNativeBodyAndCompleteConditions(t *testing.T) {
	for _, protocol := range []string{"openai.image.generate", "openai.image.edit", "gemini.image", "gemini.chat.image", "sora.video", "newapi.video", "modelark.video"} {
		t.Run(protocol, func(t *testing.T) {
			selected, err := platformintegration.SelectModel(studioNativeFixtureModel(t, protocol), "choice")
			if err != nil {
				t.Fatal(err)
			}
			before, _ := json.Marshal(selected)
			params := map[string]any{"/seed": float64(0), "/metadata": nil}
			if selected.Kind == "video" {
				params["/generate_audio"] = false
			} else {
				params["/generationConfig/responseModalities"] = []any{"TEXT", "IMAGE"}
			}
			config := map[string]any{"channelId": StudioPlatformChannelID, "model": selected.Name, "nativeVariantId": selected.ID, "nativeParameters": params}
			req := CreateTaskRequest{Prompt: "a square", Input: map[string]any{"config": config}}
			raw, err := (&Service{}).studioMediaBody(t.Context(), "user", req, selected)
			if err != nil {
				t.Fatal(err)
			}
			var body map[string]any
			_ = json.Unmarshal(raw, &body)
			if body["seed"] != float64(0) || body["metadata"] != nil || strings.Contains(string(raw), `"async"`) || strings.Contains(string(raw), "nativeParameters") {
				t.Fatalf("native values changed: %s", raw)
			}
			if selected.Kind == "video" && body["generate_audio"] != false {
				t.Fatal("lost false")
			}
			after, _ := json.Marshal(selected)
			if string(before) != string(after) {
				t.Fatal("mutated frozen catalog")
			}
			params["/model"] = "injected"
			if _, err := (&Service{}).studioMediaBody(t.Context(), "user", req, selected); err == nil {
				t.Fatal("parameter injected model")
			}
			delete(params, "/model")
			if selected.Kind == "video" {
				params["/duration"] = 8
				if _, err := (&Service{}).studioMediaBody(t.Context(), "user", req, selected); err == nil {
					t.Fatal("conditional schema ignored")
				}
				params["/resolution"] = "1080p"
				if _, err := (&Service{}).studioMediaBody(t.Context(), "user", req, selected); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestStudioNativeReferenceUsesPointerRoleAndOwnedMIME(t *testing.T) {
	s, db, _, _ := creationTestService(t)
	if err := db.AutoMigrate(&model.StudioMaterialReference{}); err != nil {
		t.Fatal(err)
	}
	resource := model.Resource{ID: "ref", UserID: "user", Provider: "platform", Kind: "image", Status: model.ResourceStatusReady, MimeType: "image/png", Size: 12, Width: 16, Height: 16}
	if err := db.Create(&resource).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.StudioMaterialReference{ResourceID: resource.ID, UserID: "user", AssetID: "asset", VersionID: "version", FileID: "file"}).Error; err != nil {
		t.Fatal(err)
	}
	one := int64(1)
	for _, tc := range []struct{ protocol, pointer, role string }{{"gemini.image", "/contents", "reference_image"}, {"gemini.chat.image", "/messages", "reference_image"}, {"modelark.video", "/content", "first_frame"}, {"openai.image.edit", "/image", "reference_image"}} {
		t.Run(tc.protocol, func(t *testing.T) {
			selected, err := platformintegration.SelectModel(studioNativeFixtureModel(t, tc.protocol), "")
			if err != nil {
				t.Fatal(err)
			}
			selected.NativeRequest.References = []platformintegration.NativeReferenceSlot{{ID: "display-id-not-path", Pointer: tc.pointer, Role: tc.role, Transport: "content-array", MinItems: &one, MaxItems: &one, MIMETypes: []string{"image/png"}}}
			if tc.protocol == "openai.image.edit" {
				selected.NativeRequest.References[0].Transport = "image-reference-array"
				selected.NativeRequest.ContentTypes = []string{"multipart/form-data"}
				var schema map[string]any
				_ = json.Unmarshal(selected.NativeRequest.Schema, &schema)
				schema["properties"].(map[string]any)["image"] = map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "minItems": 1}
				schema["required"] = []string{"model", "prompt", "image"}
				selected.NativeRequest.Schema, _ = json.Marshal(schema)
			}
			ref := map[string]any{"storageKey": "resource:ref", "role": tc.role, "url": "https://not-used.example/image", "type": "image/jpeg"}
			req := CreateTaskRequest{Prompt: "same prompt", Input: map[string]any{"config": map[string]any{"channelId": StudioPlatformChannelID}, "referenceImages": []any{ref}}}
			raw, err := s.studioMediaBody(context.Background(), "user", req, selected)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(raw), "material://asset/version") || strings.Contains(string(raw), "display-id-not-path") || strings.Contains(string(raw), "not-used") || strings.Count(string(raw), "same prompt") != 1 {
				t.Fatalf("incorrect reference: %s", raw)
			}
			if tc.protocol == "openai.image.edit" && !strings.Contains(string(raw), `"image":["material://asset/version"]`) {
				t.Fatal("edit must contain ordered exact material versions")
			}
			if tc.protocol == "gemini.image" && !strings.Contains(string(raw), `"mimeType":"image/png"`) {
				t.Fatal("lost actual MIME")
			}
			if _, err := s.studioMediaBody(t.Context(), "other", req, selected); err == nil {
				t.Fatal("cross-user reference admitted")
			}
			req.Input["referenceImages"] = []any{ref, ref}
			if _, err := s.studioMediaBody(t.Context(), "user", req, selected); err == nil {
				t.Fatal("max_items ignored")
			}
			req.Input["referenceImages"] = []any{}
			if _, err := s.studioMediaBody(t.Context(), "user", req, selected); err == nil {
				t.Fatal("min_items ignored")
			}
		})
	}
}

func TestStudioNativeParametersSurviveCanvasContract(t *testing.T) {
	a := cloudAgentMediaArgs{Mode: "video", NativeVariantID: "choice", NativeParameters: `{"/seed":0,"/generate_audio":false,"/metadata":null,"/extensions":["a"]}`}
	options := cloudAgentMediaOptions(a)
	config := options.TaskConfig()
	var expected map[string]any
	_ = json.Unmarshal([]byte(a.NativeParameters), &expected)
	if !reflect.DeepEqual(config["nativeParameters"], expected) || config["nativeVariantId"] != "choice" {
		t.Fatalf("lost contract: %#v", config)
	}
	var restored cloudAgentMediaArgs
	applyCloudAgentMediaDraftOptions(&restored, options)
	if restored.NativeParameters != a.NativeParameters || restored.NativeVariantID != a.NativeVariantID {
		t.Fatal("lost Agent draft parameters")
	}
}

func TestStudioNativeDimensionsComparePublishedPointers(t *testing.T) {
	s, db, _, _ := creationTestService(t)
	if err := db.AutoMigrate(&model.StudioMaterialReference{}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"first", "last"} {
		if err := db.Create(&model.Resource{ID: id, UserID: "user", Provider: "platform", Kind: "image", Status: model.ResourceStatusReady, MimeType: "image/png", Size: 12, Width: 16, Height: 16}).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&model.StudioMaterialReference{ResourceID: id, UserID: "user", AssetID: id, VersionID: "version", FileID: "file"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	v := &platformintegration.NativeRequestVariant{Protocol: "newapi.video", References: []platformintegration.NativeReferenceSlot{
		{ID: "first-id", Pointer: "/image", Transport: "url", Role: "first_frame"},
		{ID: "last-id", Pointer: "/metadata/image_tail", Transport: "url", Role: "last_frame", SameDimensionsAs: "/image"},
	}}
	input := canvasGenerationInput{ReferenceImages: []providerMedia{{StorageKey: "resource:first", Role: "first_frame"}, {StorageKey: "resource:last", Role: "last_frame"}}}
	if err := s.studioNativeMediaReferences(t.Context(), "user", v, input, map[string]any{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.Resource{}).Where("id = ?", "last").Update("width", 32).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.studioNativeMediaReferences(t.Context(), "user", v, input, map[string]any{}); err == nil {
		t.Fatal("different dimensions admitted")
	}
}
