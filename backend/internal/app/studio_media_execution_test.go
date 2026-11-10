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

func TestStudioNativeMediaBodyUsesPublishedBindings(t *testing.T) {
	for _, tc := range []struct {
		kind                                  string
		route                                 platformclient.MediaRoute
		binding, path, configKey, configValue string
	}{
		{"image", platformclient.GeminiMedia, "generationConfig.imageConfig.aspectRatio", "generationConfig", "size", "1:1"},
		{"image", platformclient.GeminiMedia, "/generationConfig/imageConfig/aspectRatio", "generationConfig", "size", "1:1"},
		{"video", platformclient.VideoGeneration, "aspect_ratio", "aspect_ratio", "size", "1:1"},
		{"video", platformclient.DoubaoVideoGeneration, "ratio", "ratio", "size", "1:1"},
		{"video", platformclient.DoubaoVideoGeneration, "/ratio", "ratio", "size", "1:1"},
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

func TestStudioNativeParameterPointersPreserveValues(t *testing.T) {
	body := map[string]any{"model": "exact", "prompt": "cat"}
	for _, binding := range []struct {
		path  string
		value any
	}{
		{"/generationConfig/imageConfig/aspectRatio", "1:1"},
		{"/generationConfig/seed", 0},
		{"/generate_audio", false},
		{"/metadata/a~1b/~0tag", nil},
		{"/stop", []any{"a", "b"}},
	} {
		if err := studioSetNativeField(body, binding.path, binding.value); err != nil {
			t.Fatalf("%s: %v", binding.path, err)
		}
	}
	want := map[string]any{
		"model": "exact", "prompt": "cat", "generate_audio": false,
		"generationConfig": map[string]any{"imageConfig": map[string]any{"aspectRatio": "1:1"}, "seed": 0},
		"metadata":         map[string]any{"a/b": map[string]any{"~tag": nil}},
		"stop":             []any{"a", "b"},
	}
	if !reflect.DeepEqual(body, want) {
		t.Fatalf("published native values changed: %#v", body)
	}
}

func TestStudioNativeParameterPathsRejectInvocationAndConflicts(t *testing.T) {
	for _, path := range []string{"", "/", "/bad~2key", "/bad~", "/generationConfig//seed", "/model", "/prompt", "/contents/0/parts", "/content/0", "/messages/0/content", "/image", "/images", "/mask", "/input_reference", "/reference_images", "/first_frame_image", "/last_frame_image", "/generationConfig/0/value", "/a/b/c/d/e/f/g", "generationConfig..seed"} {
		t.Run(path, func(t *testing.T) {
			body := map[string]any{}
			if studioSetNativeField(body, path, false) == nil {
				t.Fatal("unsafe or unsupported path accepted")
			}
			if len(body) != 0 {
				t.Fatalf("invalid path partially changed request: %#v", body)
			}
		})
	}
	for _, original := range []any{false, 0, nil, "value", []any{1}} {
		body := map[string]any{"existing": original}
		if studioSetNativeField(body, "/existing/child", 1) == nil || !reflect.DeepEqual(body["existing"], original) {
			t.Fatalf("replaced existing native value %#v", original)
		}
		if studioSetNativeField(body, "/existing", 1) == nil || !reflect.DeepEqual(body["existing"], original) {
			t.Fatalf("duplicate binding replaced %#v", original)
		}
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
