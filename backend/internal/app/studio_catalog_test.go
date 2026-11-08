package app

import (
	"encoding/json"
	"testing"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"yingce/backend/internal/platformintegration"
)

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
