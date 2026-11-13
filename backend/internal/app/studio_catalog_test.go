package app

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"yingce/backend/internal/platformintegration"
)

func TestStudioCatalogWirePreservesPresentationGroupsAndNativeExtensions(t *testing.T) {
	body := []byte(`{"success":true,"data":{"items":[{"model_name":"b","model_series":"GPT","presentation":{"tags":["精细编辑"],"health":{"state":"no_data","success_rate":null}},"media_generation_profile":{"future":{"exact":9007199254740993}}},{"model_name":"a","logical_model":"Image","price":{"display_items":[{"amount":"0.00000000123456789"}]}}],"groups":[{"key":"g","model_names":["a","b"],"health":{"sample_count":101,"success_count":100,"success_rate":99.01}}]}}`)
	models, groups, err := studioCatalogWire(&platformclient.Response{Body: body}, []platformclient.Model{{ModelName: "a"}, {ModelName: "b"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(models["b"]), "9007199254740993") || !strings.Contains(string(models["a"]), "0.00000000123456789") {
		t.Fatal("wire precision or exact ID mapping lost")
	}
	catalog := PublicChannelCatalog{PlatformGroups: groups, Models: []PublicChannelModel{{Platform: models["b"]}}}
	raw, err := json.Marshal(catalog)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"model_series":"GPT"`, `"tags":["精细编辑"]`, `"success_rate":null`, `"success_rate":99.01`, `9007199254740993`} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("lost public display contract: %s", want)
		}
	}
}

func TestStudioCatalogWireRejectsMismatchedOrHiddenMembers(t *testing.T) {
	for _, body := range []string{
		`{"success":false,"data":{"items":[{"model_name":"a"}]}}`,
		`{"success":true,"data":{"items":[{"model_name":"b"}]}}`,
		`{"success":true,"data":{"items":[{"model_name":"a"},{"model_name":"a"}]}}`,
		`{"success":true,"data":{"items":[{"model_name":"a"}],"groups":[{"model_names":["hidden"]}]}}`,
	} {
		if _, _, err := studioCatalogWire(&platformclient.Response{Body: []byte(body)}, []platformclient.Model{{ModelName: "a"}}); err == nil {
			t.Fatal("accepted inconsistent catalog")
		}
	}
}

func TestStudioCatalogUsesInstalledOACProtocols(t *testing.T) {
	for _, tc := range []struct {
		p              platformclient.TextProtocol
		protocol, wire string
	}{
		{platformclient.ChatCompletions, "chat-completion", "chat-completion"},
		{platformclient.Responses, "openai-response", "responses"},
		{platformclient.Messages, "claude-api", "claude-api"},
	} {
		if studioOACProtocol(platformintegration.ModelSelection{TextProtocol: tc.p}) != tc.protocol || studioTextWire(tc.p) != tc.wire {
			t.Fatalf("incorrect OAC mapping for %s", tc.p)
		}
	}
	for route, want := range map[platformclient.MediaRoute]string{platformclient.ImageGeneration: "openai-image", platformclient.GeminiMedia: "gemini-image", platformclient.VideoGeneration: "newapi", platformclient.DoubaoVideoGeneration: "volcengine-ark-video"} {
		if studioOACProtocol(platformintegration.ModelSelection{MediaRoute: route}) != want {
			t.Fatalf("incorrect media mapping for %s", route)
		}
	}
}

func TestStudioCatalogProjectionKeepsTypedDefaultsAndMissingLimits(t *testing.T) {
	s := platformintegration.ModelSelection{Kind: "image", NativeEndpoint: platformintegration.NativeEndpoint{Schema: json.RawMessage(`{"properties":{"size":{"type":"string","enum":["1024x1024"]},"n":{"type":"integer","maximum":3,"default":1},"seed":{"type":"integer","minimum":0},"watermark":{"type":"boolean","default":false}}}`)}}
	p := studioCapabilityProjection(s)["image"].(map[string]any)
	params := p["parameters"].([]map[string]any)
	for _, param := range params {
		if param["name"] == "seed" {
			if _, ok := param["default"]; ok {
				t.Fatal("invented numeric default")
			}
		}
		if param["name"] == "watermark" && param["default"] != false {
			t.Fatal("lost false default")
		}
	}
	if p["references"].(map[string]any)["maxImages"] != 0 {
		t.Fatal("invented reference support")
	}
	s.Kind = "video"
	v := studioCapabilityProjection(s)["video"].(map[string]any)
	if v["generateAudio"].(map[string]any)["default"] != false {
		t.Fatal("non-boolean audio default")
	}
	if v["duration"].(map[string]any)["default"] != float64(0) {
		t.Fatal("non-numeric duration default")
	}
}

func TestStudioDoubaoCatalogProjectsNativeRatioAndFourSecond720p(t *testing.T) {
	selected := platformintegration.ModelSelection{Kind: "video", MediaRoute: platformclient.DoubaoVideoGeneration, NativeEndpoint: platformintegration.NativeEndpoint{
		Schema:   json.RawMessage(`{"properties":{"ratio":{"type":"string","enum":["16:9","1:1","adaptive"]},"resolution":{"type":"string","enum":["480p","720p"]},"duration":{"type":"integer","enum":[-1,4,5,6,7,8,9,10,11,12,13,14,15]}}}`),
		Defaults: map[string]any{"ratio": "adaptive", "resolution": "720p"},
	}}
	video := studioCapabilityProjection(selected)["video"].(map[string]any)
	if video["defaultRatio"] != "adaptive" || len(video["ratios"].([]any)) != 3 {
		t.Fatalf("native ratio disappeared from the controls: %+v", video)
	}
	if video["defaultResolution"] != "720p" || video["duration"].(map[string]any)["values"].([]any)[1] != float64(4) {
		t.Fatalf("published 720p/4 seconds changed: %+v", video)
	}
}

func TestStudioCatalogProjectsConstAsSinglePublishedChoice(t *testing.T) {
	selected := platformintegration.ModelSelection{Kind: "video", NativeEndpoint: platformintegration.NativeEndpoint{
		Schema: json.RawMessage(`{"type":"object","properties":{"resolution":{"type":"string","const":"720p"},"ratio":{"type":"string","const":"16:9"},"duration":{"type":"integer","const":4},"generate_audio":{"type":"boolean","const":false}}}`),
	}}
	video := studioCapabilityProjection(selected)["video"].(map[string]any)
	for field, want := range map[string]string{"resolutions": "720p", "ratios": "16:9"} {
		values := video[field].([]any)
		if len(values) != 1 || values[0] != want {
			t.Errorf("const disappeared from %s: %+v", field, values)
		}
	}
	if video["defaultResolution"] != "720p" || video["defaultRatio"] != "16:9" || video["duration"].(map[string]any)["default"] != float64(4) {
		t.Errorf("missing const defaults: %+v", video)
	}
	selected.Kind = "image"
	image := studioCapabilityProjection(selected)["image"].(map[string]any)
	for _, param := range image["parameters"].([]map[string]any) {
		if param["name"] == "imageResolution" && (param["default"] != "720p" || len(param["values"].([]any)) != 1) {
			t.Errorf("image parameter lost fixed resolution: %+v", param)
		}
		if param["name"] == "generate_audio" && param["default"] != false {
			t.Errorf("lost false const: %+v", param)
		}
	}
}

func TestStudioMediaConstSpecificationsUsePublishedValuesAndRejectConflicts(t *testing.T) {
	selected := platformintegration.ModelSelection{Kind: "video", NativeEndpoint: platformintegration.NativeEndpoint{
		Schema: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"resolution":{"type":"string","const":"720p"},"duration":{"type":"integer","const":4},"generate_audio":{"type":"boolean","const":false}},"required":["resolution","duration","generate_audio"]}`),
	}}
	for _, value := range []string{"", "720p", "1080p"} {
		t.Run(value, func(t *testing.T) {
			input := canvasGenerationInput{}
			input.Config.VQuality = value
			spec, err := studioMediaSpecifications(input, selected)
			if value == "1080p" {
				if err == nil {
					t.Fatal("explicit conflicting resolution was silently replaced")
				}
				return
			}
			if err != nil || spec["resolution"] != "720p" || spec["duration"] != float64(4) || spec["generate_audio"] != false {
				t.Fatalf("published const types/values lost: %+v %v", spec, err)
			}
		})
	}
}
