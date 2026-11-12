package app

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"yingce/backend/internal/platformintegration"
)

// These fixtures are actual Gateway compiled profiles passed through Platform's
// production catalog projection. Only the outer model identity/price is free
// test data; Studio must not synthesize dimensions while consuming the result.
func TestStudioImageOutputSizeFromPublishedPlatformProjection(t *testing.T) {
	for _, name := range []string{"gpt-default", "gpt-4mp", "gemini"} {
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile("../platformintegration/testdata/image-output-size/" + name + ".model.json")
			if err != nil {
				t.Fatal(err)
			}
			var model platformclient.Model
			if err = json.Unmarshal(raw, &model); err != nil {
				t.Fatal(err)
			}
			selected, err := platformintegration.SelectModel(model, "")
			if err != nil {
				t.Fatal(err)
			}
			before, _ := json.Marshal(selected)
			if len(selected.NativeRequest.SizePresets) == 0 {
				t.Fatal("lost actual published presets")
			}
			for _, preset := range selected.NativeRequest.SizePresets {
				params := map[string]any{}
				for pointer := range selected.Bindings {
					if value, exists := platformintegration.NativeValue(preset.Values, pointer); exists {
						params[pointer] = value
					}
				}
				req := CreateTaskRequest{Prompt: "A still life", Input: map[string]any{"config": map[string]any{"channelId": StudioPlatformChannelID, "model": selected.Name, "nativeVariantId": selected.ID, "nativeParameters": params}}}
				body, err := (&Service{}).studioMediaBody(t.Context(), "user", req, selected)
				if err != nil {
					t.Fatalf("%s: %v", preset.ID, err)
				}
				// SDK preparation and Studio's existing pointer builder produce the same
				// bytes. Both preview and submission receive this original prepared body.
				variant, _ := json.Marshal(selected.NativeRequest)
				prepared, err := platformclient.PrepareImageSize(variant, body, platformclient.ImageSizeSelection{PresetID: preset.ID})
				if err != nil || string(body) != string(prepared) {
					t.Fatalf("body diverged for %s: %v", preset.ID, err)
				}
				if name == "gemini" {
					var value map[string]any
					_ = json.Unmarshal(body, &value)
					if _, ok := value["size"]; ok {
						t.Fatal("invented OpenAI size in Gemini request")
					}
					if preset.Width != 0 || preset.Height != 0 {
						t.Fatal("invented exact Gemini output pixels")
					}
				}
			}
			after, _ := json.Marshal(selected)
			if string(before) != string(after) {
				t.Fatal("mutated published model contract")
			}
			// Round-trip the consumer view without dropping either the envelope or its
			// underlying complete alternatives. The exact schemas remain authoritative.
			var roundtrip platformintegration.ModelSelection
			if json.Unmarshal(after, &roundtrip) != nil {
				t.Fatal("invalid model projection")
			}
			var originalAlternatives, restoredAlternatives any
			_ = json.Unmarshal(selected.NativeRequest.SupplyAlternatives, &originalAlternatives)
			_ = json.Unmarshal(roundtrip.NativeRequest.SupplyAlternatives, &restoredAlternatives)
			if !reflect.DeepEqual(selected.NativeRequest.OutputSize, roundtrip.NativeRequest.OutputSize) || !reflect.DeepEqual(originalAlternatives, restoredAlternatives) {
				t.Fatal("lost output size metadata")
			}
		})
	}
}

func TestStudioImageOutputSizeRejectsOverAreaAndUnalignedNativeBody(t *testing.T) {
	raw, err := os.ReadFile("../platformintegration/testdata/image-output-size/gpt-4mp.model.json")
	if err != nil {
		t.Fatal(err)
	}
	var model platformclient.Model
	if json.Unmarshal(raw, &model) != nil {
		t.Fatal("bad fixture")
	}
	selected, err := platformintegration.SelectModel(model, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		size string
		want bool
	}{{"2048x2048", true}, {"2560x1440", true}, {"2064x2048", false}, {"1025x1024", false}, {"1024.5x1024", false}, {"auto", false}} {
		req := CreateTaskRequest{Prompt: "A still life", Input: map[string]any{"config": map[string]any{"channelId": StudioPlatformChannelID, "model": selected.Name, "nativeVariantId": selected.ID, "nativeParameters": map[string]any{"/size": tc.size, "/n": 10}}}}
		_, err := (&Service{}).studioMediaBody(t.Context(), "user", req, selected)
		if (err == nil) != tc.want {
			t.Fatalf("%s: allowed=%v error=%v", tc.size, tc.want, err)
		}
	}
}
