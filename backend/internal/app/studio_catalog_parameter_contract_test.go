package app

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"yingce/backend/internal/platformintegration"
)

func TestStudioPublishedParameterMultipleOfIsNotRelativeStep(t *testing.T) {
	selected := platformintegration.ModelSelection{Kind: "image", NativeEndpoint: platformintegration.NativeEndpoint{
		Schema: json.RawMessage(`{"type":"object","properties":{"strength":{"type":"number","minimum":0.3,"maximum":0.9,"multipleOf":0.2,"default":0.4},"duration":{"type":"number","minimum":3,"maximum":9,"multipleOf":2,"default":4}}}`),
	}}
	image := studioCapabilityProjection(selected)["image"].(map[string]any)
	for _, p := range image["parameters"].([]map[string]any) {
		if p["name"] == "strength" && (p["multipleOf"] != 0.2 || p["default"] != 0.4) {
			t.Errorf("lost published multipleOf/default: %+v", p)
		}
	}
	selected.Kind = "video"
	duration := studioCapabilityProjection(selected)["video"].(map[string]any)["duration"].(map[string]any)
	if duration["multipleOf"] != float64(2) || duration["min"] != float64(3) || duration["max"] != float64(9) {
		t.Fatalf("video range lost schema semantics: %+v", duration)
	}
}

func TestStudioPublishedAudioAliasControlsMatchNativeBody(t *testing.T) {
	for _, field := range []string{"audio", "generate_audio"} {
		t.Run(field, func(t *testing.T) {
			selected := platformintegration.ModelSelection{Name: "published-video", Kind: "video", MediaRoute: platformclient.DoubaoVideoGeneration, NativeEndpoint: platformintegration.NativeEndpoint{
				Schema: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"` + field + `":{"type":"boolean","default":true}},"required":["` + field + `"]}`), Bindings: map[string]string{field: "generate_audio"},
			}}
			audio := studioCapabilityProjection(selected)["video"].(map[string]any)["generateAudio"].(map[string]any)
			if audio["supported"] != true || audio["default"] != true {
				t.Errorf("published audio absent from controls: %+v", audio)
			}
			for _, override := range []string{"", "false", "true"} {
				body, err := (&Service{}).studioMediaBody(context.Background(), "user", CreateTaskRequest{Prompt: "fixture", Input: map[string]any{"config": map[string]any{"channelId": StudioPlatformChannelID, "videoGenerateAudio": override}}}, selected)
				if err != nil {
					t.Fatal(err)
				}
				var native map[string]any
				_ = json.Unmarshal(body, &native)
				if native["generate_audio"] != (override != "false") {
					t.Fatalf("audio UI and native binding disagree: %s", body)
				}
			}
		})
	}
}
