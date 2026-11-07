package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"yingce/backend/internal/model"
	"yingce/backend/internal/platformintegration"
)

func TestStudioNativeMediaBodyUsesPublishedBindings(t *testing.T) {
	for _, tc := range []struct {
		kind                                  string
		route                                 platformclient.MediaRoute
		binding, path, configKey, configValue string
	}{
		{"image", platformclient.GeminiMedia, "generationConfig.imageConfig.aspectRatio", "generationConfig", "size", "1:1"},
		{"video", platformclient.VideoGeneration, "aspect_ratio", "aspect_ratio", "size", "1:1"},
		{"video", platformclient.DoubaoVideoGeneration, "ratio", "ratio", "size", "1:1"},
	} {
		t.Run(string(tc.route), func(t *testing.T) {
			selected := platformintegration.ModelSelection{Name: "exact", Kind: tc.kind, MediaRoute: tc.route, NativeEndpoint: platformintegration.NativeEndpoint{Schema: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"aspect_ratio":{"type":"string","enum":["1:1"]}},"required":["aspect_ratio"]}`), Bindings: map[string]string{"aspect_ratio": tc.binding}}}
			config := map[string]any{"channelId": StudioPlatformChannelID, "model": "exact", tc.configKey: tc.configValue}
			if tc.kind == "image" {
				config["imageParameters"] = map[string]any{"size": "1:1"}
			}
			body, err := (&Service{}).studioMediaBody(context.Background(), "user", CreateTaskRequest{Prompt: "cat", Input: map[string]any{"config": config}}, selected)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(body), tc.path) || !strings.Contains(string(body), "cat") {
				t.Fatalf("lost native binding: %s", body)
			}
			if tc.route == platformclient.GeminiMedia && !strings.Contains(string(body), `"aspectRatio":"1:1"`) {
				t.Fatal("Gemini binding was flattened")
			}
		})
	}
}

func TestStudioMediaReferencesPinOwnedVersions(t *testing.T) {
	s, db, _, _ := creationTestService(t)
	if err := db.AutoMigrate(&model.StudioMaterialReference{}); err != nil {
		t.Fatal(err)
	}
	res := model.Resource{ID: "resource-1", UserID: "user", Provider: "platform", Kind: "image", Status: model.ResourceStatusReady, MimeType: "image/png", Size: 12}
	if err := db.Create(&res).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.StudioMaterialReference{ResourceID: res.ID, UserID: "user", AssetID: "asset-fixed", VersionID: "version-fixed", FileID: "file-fixed"}).Error; err != nil {
		t.Fatal(err)
	}
	selected := platformintegration.ModelSelection{Name: "image", Kind: "image", MediaRoute: platformclient.ImageEdit, NativeEndpoint: platformintegration.NativeEndpoint{References: []platformintegration.NativeReference{{Field: "images", Transport: "image-reference-array", Roles: []string{"reference_image"}, MinItems: 1, MaxItems: 1, MaxItemBytes: 20, MIMETypes: []string{"image/png"}, ImageFormat: "openai"}}}}
	input := canvasGenerationInput{ReferenceImages: []providerMedia{{StorageKey: "resource:resource-1", Role: "reference", URL: "https://untrusted.test/ignored"}}}
	body := map[string]any{}
	if err := s.studioMediaReferences(context.Background(), "user", selected, input, body); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(body)
	if string(raw) != `{"images":[{"image_url":"material://asset-fixed/version-fixed"}]}` {
		t.Fatalf("material identity changed: %s", raw)
	}
	if s.studioMediaReferences(context.Background(), "other", selected, input, map[string]any{}) == nil {
		t.Fatal("cross-account reference admitted")
	}
	input.ReferenceImages = append(input.ReferenceImages, input.ReferenceImages[0])
	if s.studioMediaReferences(context.Background(), "user", selected, input, map[string]any{}) == nil {
		t.Fatal("reference count exceeded")
	}
	input.ReferenceImages = []providerMedia{{URL: "https://external.test"}}
	if s.studioMediaReferences(context.Background(), "user", selected, input, map[string]any{}) == nil {
		t.Fatal("external reference bypassed materials")
	}
}
