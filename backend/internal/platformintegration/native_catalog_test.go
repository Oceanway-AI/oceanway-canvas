package platformintegration

import (
	"encoding/json"
	"testing"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
)

func nativeCatalogModel(protocol, path, kind string) platformclient.Model {
	variant := map[string]any{
		"id": "published-choice", "method": "POST", "gateway_protocol": protocol,
		"specifications_schema": map[string]any{"type": "object", "properties": map[string]any{}},
		"native_request_variant": map[string]any{
			"id": "native-choice", "protocol": protocol, "operation": "create", "method": "POST", "path": path,
			"content_types": []string{"application/json"}, "modes": []string{"sync", "async"}, "max_outputs": 1,
			"request_schema": map[string]any{"type": "object"}, "defaults": map[string]any{},
		},
	}
	raw, _ := json.Marshal(map[string]any{"default_variant": "published-choice", "variants": []any{variant}})
	return platformclient.Model{ModelName: "arbitrary-public-id", ModelType: kind, InvocationStatus: "available", Price: json.RawMessage(`{"status":"available","version":"price1"}`), MediaGenerationProfile: raw}
}

func TestNativePublishedProtocolsUseNamedSDKRoutes(t *testing.T) {
	for _, tc := range []struct {
		protocol, path, kind string
		route                platformclient.MediaRoute
	}{
		{"openai.image.generate", "/v1/images/generations", "image", platformclient.ImageGeneration},
		{"openai.image.edit", "/v1/images/edits", "image", platformclient.ImageEdit},
		{"gemini.image", "/v1beta/models/{model}:generateContent", "image", platformclient.GeminiMedia},
		{"gemini.chat.image", "/v1/chat/completions", "image", platformclient.GeminiChatImage},
		{"sora.video", "/v1/videos", "video", platformclient.VideoGeneration},
		{"newapi.video", "/v1/video/generations", "video", platformclient.NewAPIVideoGeneration},
		{"modelark.video", "/api/v3/contents/generations/tasks", "video", platformclient.ModelArkVideoGeneration},
	} {
		t.Run(tc.protocol, func(t *testing.T) {
			m := nativeCatalogModel(tc.protocol, tc.path, tc.kind)
			s, err := SelectModel(m, "")
			if err != nil || s.MediaRoute != tc.route || s.ID != "published-choice" {
				t.Fatalf("route: %+v %v", s, err)
			}
			m = nativeCatalogModel(tc.protocol, "/wrong", tc.kind)
			if _, err := SelectModel(m, ""); err == nil {
				t.Fatal("wrong native path admitted")
			}
		})
	}
}

func TestNativeCatalogRejectsIncompleteOrUnsupportedContract(t *testing.T) {
	for _, change := range []map[string]any{
		{"modes": []string{"stream"}}, {"content_types": []string{"multipart/form-data"}},
		{"unresolved_conditions": []string{"unknown"}}, {"operation": "delete"}, {"request_schema": nil},
		{"defaults": map[string]any{"prompt": "hidden"}}, {"defaults": map[string]any{"model": "other"}},
	} {
		m := nativeCatalogModel("modelark.video", "/api/v3/contents/generations/tasks", "video")
		var p map[string]any
		_ = json.Unmarshal(m.MediaGenerationProfile, &p)
		v := p["variants"].([]any)[0].(map[string]any)["native_request_variant"].(map[string]any)
		for k, value := range change {
			v[k] = value
		}
		m.MediaGenerationProfile, _ = json.Marshal(p)
		if _, err := SelectModel(m, ""); err == nil {
			t.Fatalf("accepted %v", change)
		}
	}
}
