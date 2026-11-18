package app

import (
	"testing"

	"yingce/backend/internal/model"
)

func TestNormalizeVideoAudioPriceSelector(t *testing.T) {
	for _, test := range []struct {
		value string
		want  string
	}{
		{value: "true", want: "true"},
		{value: "false", want: "false"},
		{value: " FALSE ", want: "false"},
		{value: "*", want: ""},
	} {
		t.Run(test.value, func(t *testing.T) {
			selector, _, _, err := normalizeChannelModelVariantSelector("video", ChannelModelVariantRequest{
				Selector: map[string]string{"videoGenerateAudio": test.value},
			})
			if err != nil || selector["videoGenerateAudio"] != test.want {
				t.Fatalf("selector = %#v, error = %v, want audio %q", selector, err, test.want)
			}
			if test.want == "" && len(selector) != 0 {
				t.Fatalf("wildcard must share the default selector, got %#v", selector)
			}
		})
	}
	for _, value := range []string{"yes", "1", "0", "auto"} {
		if _, _, _, err := normalizeChannelModelVariantSelector("video", ChannelModelVariantRequest{
			Selector: map[string]string{"videoGenerateAudio": value},
		}); err == nil {
			t.Fatalf("invalid audio selector %q accepted", value)
		}
	}
	for _, capability := range []string{"text", "image", "audio"} {
		for _, value := range []string{"true", "false", "*"} {
			if _, _, _, err := normalizeChannelModelVariantSelector(capability, ChannelModelVariantRequest{
				Selector: map[string]string{"videoGenerateAudio": value},
			}); err == nil {
				t.Fatalf("%s model accepted video audio selector %q", capability, value)
			}
		}
	}
}

func TestVideoVariantMatchesAudioAndActualReferences(t *testing.T) {
	channelModel := model.ChannelModel{Variants: []model.ChannelModelVariant{
		{ID: "default", SelectorJSON: `{}`, Enabled: true},
		{ID: "reference", SelectorJSON: `{"operation":"video_to_video"}`, Enabled: true},
		{ID: "reference-audio", SelectorJSON: `{"operation":"video_to_video","videoGenerateAudio":"true"}`, Enabled: true},
		{ID: "reference-silent", SelectorJSON: `{"operation":"video_to_video","videoGenerateAudio":"false"}`, Enabled: true},
	}}
	for _, test := range []struct {
		name  string
		value any
		want  string
	}{
		{name: "audio boolean", value: true, want: "reference-audio"},
		{name: "silent boolean", value: false, want: "reference-silent"},
		{name: "audio string", value: "true", want: "reference-audio"},
		{name: "silent string", value: "false", want: "reference-silent"},
		{name: "unspecified", want: "reference"},
		{name: "invalid is not silent", value: "invalid", want: "reference"},
	} {
		t.Run(test.name, func(t *testing.T) {
			options := map[string]any{}
			if test.value != nil {
				options["videoGenerateAudio"] = test.value
			}
			for _, source := range []string{"config", "capabilityOptions"} {
				intent := ModelRequestIntentFromTaskInput(map[string]any{
					"mode": "video", "referenceVideos": []any{map[string]any{"id": "video-reference"}}, source: options,
				}, "canvas_video", "reference_to_video")
				tier := channelModelVariantForIntent(channelModel, intent)
				if tier == nil || tier.ID != test.want {
					t.Fatalf("%s matched %#v, want %s", source, tier, test.want)
				}
			}
		})
	}
	intent := ModelRequestIntent{Capability: "video", Inputs: map[string]int{"video": 1}}
	intent.Options = mergeIntentDefaults(map[string]any{"videoGenerateAudio": false}, map[string]any{"videoGenerateAudio": true})
	if tier := channelModelVariantForIntent(channelModel, intent); tier == nil || tier.ID != "reference-silent" {
		t.Fatalf("explicit false must override audio default: %#v", tier)
	}
}
