package app

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"yingce/backend/internal/model"
	"yingce/backend/internal/platformintegration"
)

func TestStudioNativeRequestOmitsOptionalPublishedDefaults(t *testing.T) {
	for _, protocol := range []string{"newapi.video", "gemini.image"} {
		selected, err := platformintegration.SelectModel(studioNativeFixtureModel(t, protocol), "")
		if err != nil {
			t.Fatal(err)
		}
		params := map[string]any{}
		req := CreateTaskRequest{Prompt: "A still life", Input: map[string]any{"config": map[string]any{"channelId": StudioPlatformChannelID, "nativeParameters": params}}}
		raw, err := (&Service{}).studioMediaBody(t.Context(), "user", req, selected)
		if err != nil {
			t.Fatal(err)
		}
		var body map[string]any
		if json.Unmarshal(raw, &body) != nil {
			t.Fatal("invalid native request")
		}
		for _, key := range []string{"seed", "generate_audio", "metadata", "duration", "resolution", "ratio", "generationConfig"} {
			if _, present := body[key]; present {
				t.Fatalf("%s: optional %s was submitted: %s", protocol, key, raw)
			}
		}
		if protocol == "gemini.image" {
			params["/generationConfig/imageConfig/imageSize"] = "2K"
			raw, err = (&Service{}).studioMediaBody(t.Context(), "user", req, selected)
			if err != nil || !strings.Contains(string(raw), `"responseModalities":["IMAGE"]`) {
				t.Fatalf("lost the required sibling of an explicit Gemini setting: %s %v", raw, err)
			}
		}
	}
}

func TestStudioVideoSupplyChecksResolutionDurationAndReferencesTogether(t *testing.T) {
	one, three := int64(1), int64(3)
	slot := platformintegration.NativeReferenceSlot{ID: "images", Role: "reference_image", Pointer: "/metadata/image_urls", Transport: "url-array", MaxItems: &three}
	option720 := platformintegration.NativeSupplyAlternative{Schema: json.RawMessage(`{"type":"object","properties":{"resolution":{"const":"720p"},"duration":{"type":"integer","minimum":3,"maximum":10}},"required":["resolution","duration"]}`), References: []platformintegration.NativeReferenceSlot{slot}}
	slot.MaxItems = &one
	option1080 := platformintegration.NativeSupplyAlternative{Schema: json.RawMessage(`{"type":"object","properties":{"resolution":{"const":"1080p"},"duration":{"enum":[4,6,8,10]}},"required":["resolution","duration"]}`), References: []platformintegration.NativeReferenceSlot{slot}}
	options, _ := json.Marshal([]platformintegration.NativeSupplyAlternative{option720, option1080})
	v := &platformintegration.NativeRequestVariant{Schema: json.RawMessage(`{"type":"object"}`), SupplyAlternatives: options}
	for _, tc := range []struct {
		resolution      string
		duration, count int
		want            bool
	}{{"720p", 5, 3, true}, {"1080p", 4, 1, true}, {"1080p", 5, 1, false}, {"1080p", 4, 3, false}, {"720p", 5, 4, false}} {
		var refs []studioNativeReferenceResource
		for range tc.count {
			refs = append(refs, studioNativeReferenceResource{Role: "reference_image", Resource: &model.Resource{Kind: "image", MimeType: "image/png", Size: 12}})
		}
		_, err := studioPrepareNativeCombination(v, refs, map[string]any{"resolution": tc.resolution, "duration": tc.duration})
		if (err == nil) != tc.want {
			t.Fatalf("%+v: %v", tc, err)
		}
	}
}

func TestStudioNativeSupplyUsesOneOutputSizeAndItsReferencePolicy(t *testing.T) {
	raw, err := os.ReadFile("../platformintegration/testdata/image-output-size/gpt-default.model.json")
	if err != nil {
		t.Fatal(err)
	}
	var m platformclient.Model
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	selected, err := platformintegration.SelectModel(m, "")
	if err != nil {
		t.Fatal(err)
	}
	v := selected.NativeRequest
	fourMP, eightMP := *v.OutputSize, *v.OutputSize
	fourMP.MaxPixels, fourMP.AllowAuto = 4194304, false
	eightMP.MaxPixels, eightMP.AllowAuto = 8294400, true
	one, sixteen := int64(1), int64(16)
	slot := platformintegration.NativeReferenceSlot{ID: "images", Role: "reference_image", MinItems: &one, MaxItems: &sixteen}
	schema := json.RawMessage(`{"type":"object","required":["size"]}`)
	low := platformintegration.NativeSupplyAlternative{Schema: schema, OutputSize: &fourMP, References: []platformintegration.NativeReferenceSlot{slot}}
	high := platformintegration.NativeSupplyAlternative{Schema: schema, OutputSize: &eightMP, References: []platformintegration.NativeReferenceSlot{slot}}
	v.OutputSize = &eightMP
	for _, tc := range []struct {
		size  string
		count int
		want  bool
	}{{"2048x2048", 1, true}, {"3072x2048", 16, true}, {"auto", 1, true}, {"auto", 0, false}, {"auto", 17, false}, {"3840x3840", 1, false}} {
		v.SupplyAlternatives, _ = json.Marshal([]platformintegration.NativeSupplyAlternative{low, high})
		refs := make([]studioNativeReferenceResource, tc.count)
		for i := range refs {
			refs[i] = studioNativeReferenceResource{Role: "reference_image", Resource: &model.Resource{Kind: "image", Size: 10}}
		}
		body := map[string]any{"model": m.ModelName, "prompt": "A still life", "size": tc.size}
		prepared, err := studioPrepareNativeCombination(v, refs, body)
		if (err == nil) != tc.want {
			t.Fatalf("%+v: %s %v", tc, prepared, err)
		}
		if tc.size == "3072x2048" {
			high.References[0].MaxItems = &one
			v.SupplyAlternatives, _ = json.Marshal([]platformintegration.NativeSupplyAlternative{low, high})
			if _, err := studioPrepareNativeCombination(v, refs, body); err == nil {
				t.Fatal("used the 8MP branch's size with the 4MP branch's material limit")
			}
			high.References[0].MaxItems = &sixteen
		}
	}
}

func TestStudioNativeSuppliesOnlyUnambiguousRequiredDefaults(t *testing.T) {
	v := &platformintegration.NativeRequestVariant{Schema: json.RawMessage(`{"type":"object"}`), Defaults: map[string]any{"seed": 7}}
	a := platformintegration.NativeSupplyAlternative{Schema: json.RawMessage(`{"properties":{"duration":{"const":4},"seed":{"type":"integer"}},"required":["duration"]}`), References: []platformintegration.NativeReferenceSlot{}}
	v.SupplyAlternatives, _ = json.Marshal([]platformintegration.NativeSupplyAlternative{a})
	got, err := studioPrepareNativeCombination(v, nil, map[string]any{})
	if err != nil || string(got) != `{"duration":4}` {
		t.Fatalf("branch-required value: %s %v", got, err)
	}
	b := a
	b.Schema = json.RawMessage(`{"properties":{"duration":{"const":8}},"required":["duration"]}`)
	v.SupplyAlternatives, _ = json.Marshal([]platformintegration.NativeSupplyAlternative{a, b})
	if _, err := studioPrepareNativeCombination(v, nil, map[string]any{}); err == nil {
		t.Fatal("silently picked one of two incompatible required durations")
	}
	got, err = studioPrepareNativeCombination(v, nil, map[string]any{"duration": 8})
	if err != nil || string(got) != `{"duration":8}` {
		t.Fatalf("explicit choice lost: %s %v", got, err)
	}
	a.Schema = json.RawMessage(`{"required":["seed"]}`)
	v.SupplyAlternatives, _ = json.Marshal([]platformintegration.NativeSupplyAlternative{a})
	got, err = studioPrepareNativeCombination(v, nil, map[string]any{})
	if err != nil || string(got) != `{"seed":7}` {
		t.Fatalf("lost required key inherited from the native schema: %s %v", got, err)
	}
	for _, bad := range []string{"[]", "null", `[{"request_schema":{},"native_reference_inputs":null}]`} {
		v.SupplyAlternatives = json.RawMessage(bad)
		if _, err := studioPrepareNativeCombination(v, nil, map[string]any{}); err == nil {
			t.Fatalf("accepted malformed supply alternatives %s", bad)
		}
	}
}

func TestStudioAgentSelectsPublishedVariantFromActualReferences(t *testing.T) {
	item := studioNativeFixtureModel(t, "modelark.video")
	var profile platformintegration.MediaProfile
	if err := json.Unmarshal(item.MediaGenerationProfile, &profile); err != nil {
		t.Fatal(err)
	}
	edit := profile.Variants[0]
	edit.ID = "with-reference"
	native := *edit.NativeRequest
	one := int64(1)
	native.References = []platformintegration.NativeReferenceSlot{{ID: "image", Role: "reference_image", Pointer: "/content", Transport: "content-array", MinItems: &one, MaxItems: &one}}
	edit.NativeRequest = &native
	profile.Variants = append(profile.Variants, edit)
	ref := studioNativeReferenceResource{Role: "reference_image", Resource: &model.Resource{Kind: "image", MimeType: "image/png", Size: 100}, URI: "material://asset/version"}
	original := cloudAgentMediaArgs{Mode: "video", Prompt: "A still life"}
	for _, tc := range []struct {
		refs    []studioNativeReferenceResource
		variant string
	}{
		{nil, profile.DefaultVariant}, {[]studioNativeReferenceResource{ref}, edit.ID}, {[]studioNativeReferenceResource{ref, ref}, ""},
	} {
		args := original
		err := studioChooseAgentNativeVariant(&args, &cloudAgentRuntime{}, item, profile, tc.refs)
		if tc.variant == "" && err == nil || tc.variant != "" && (err != nil || args.NativeVariantID != tc.variant) {
			t.Fatalf("%+v selected %s: %v", tc, args.NativeVariantID, err)
		}
	}
	var other platformintegration.MediaProfile
	if err := json.Unmarshal(studioNativeFixtureModel(t, "newapi.video").MediaGenerationProfile, &other); err != nil {
		t.Fatal(err)
	}
	variant := other.Variants[0]
	variant.ID = "different-path"
	profile.Variants = append(profile.Variants, variant)
	args := original
	if err := studioChooseAgentNativeVariant(&args, &cloudAgentRuntime{}, item, profile, nil); err == nil {
		t.Fatal("silently chose between two different requests")
	}
}

func TestStudioNativeMaterialRolesAndCombinedMeasurements(t *testing.T) {
	one, totalBytes, totalDuration := int64(1), int64(20), int64(8000)
	v := &platformintegration.NativeRequestVariant{Schema: json.RawMessage(`{"type":"object"}`)}
	a := platformintegration.NativeSupplyAlternative{
		Schema:     json.RawMessage(`{"type":"object"}`),
		References: []platformintegration.NativeReferenceSlot{{ID: "video", Role: "reference_video", MaxItems: &one}, {ID: "audio", Role: "reference_audio", MaxItems: &one}},
		Materials:  &platformintegration.NativeMaterialConstraints{MaxTotalBytes: &totalBytes, MaxVideoAndOutputDurationMS: &totalDuration, AudioRequiresVisual: true, ReferenceOrder: "images_videos_audios"},
	}
	v.SupplyAlternatives, _ = json.Marshal([]platformintegration.NativeSupplyAlternative{a})
	video := studioNativeReferenceResource{Role: "reference_video", Resource: &model.Resource{Kind: "video", Size: 10, DurationMs: 4000}}
	audio := studioNativeReferenceResource{Role: "reference_audio", Resource: &model.Resource{Kind: "audio", Size: 10}}
	body := map[string]any{"seconds": "4"}
	for _, tc := range []struct {
		refs []studioNativeReferenceResource
		want bool
	}{
		{[]studioNativeReferenceResource{video, audio}, true}, {[]studioNativeReferenceResource{audio}, false}, {[]studioNativeReferenceResource{audio, video}, false},
	} {
		if _, err := studioPrepareNativeCombination(v, tc.refs, body); (err == nil) != tc.want {
			t.Fatalf("reference shape: %+v %v", tc, err)
		}
	}
	video.Resource.DurationMs++
	if _, err := studioPrepareNativeCombination(v, []studioNativeReferenceResource{video, audio}, body); err == nil {
		t.Fatal("combined video duration ignored")
	}
	video.Resource.DurationMs--
	audio.Resource.Size++
	if _, err := studioPrepareNativeCombination(v, []studioNativeReferenceResource{video, audio}, body); err == nil {
		t.Fatal("combined bytes ignored")
	}
	frame := platformintegration.NativeReferenceSlot{ID: "first", Pointer: "/content", Role: "first_frame", MinItems: &one, MaxItems: &one}
	tail := platformintegration.NativeReferenceSlot{ID: "last", Pointer: "/content", Role: "last_frame", MaxItems: &one, SameDimensionsAs: "first"}
	first := studioNativeReferenceResource{Role: frame.Role, Resource: &model.Resource{Width: 1920, Height: 1080}}
	last := studioNativeReferenceResource{Role: tail.Role, Resource: &model.Resource{Width: 1920, Height: 1080}}
	if err := studioValidateNativeReferences([]platformintegration.NativeReferenceSlot{frame, tail}, []studioNativeReferenceResource{first, last}); err != nil {
		t.Fatal(err)
	}
	last.Resource.Height--
	if err := studioValidateNativeReferences([]platformintegration.NativeReferenceSlot{frame, tail}, []studioNativeReferenceResource{first, last}); err == nil {
		t.Fatal("reference slot IDs sharing /content did not enforce equal dimensions")
	}
}

func TestStudioCatalogUsesPublishedPointersAndFrameRoles(t *testing.T) {
	one := int64(1)
	frame := platformintegration.NativeReferenceSlot{ID: "first", Pointer: "/image", Role: "first_frame", MinItems: &one, MaxItems: &one}
	selected := platformintegration.ModelSelection{Kind: "video", NativeEndpoint: platformintegration.NativeEndpoint{
		Schema:        json.RawMessage(`{"properties":{"/metadata/resolution":{"const":"720p"},"/metadata/aspect_ratio":{"enum":["16:9"]},"/seconds":{"enum":["4","6"]},"/metadata/generate_audio":{"type":"boolean"}}}`),
		NativeRequest: &platformintegration.NativeRequestVariant{References: []platformintegration.NativeReferenceSlot{frame}},
	}}
	projected := studioCapabilityProjection(selected)["video"].(map[string]any)
	refs := projected["references"].(map[string]any)
	if refs["maxImages"] != int64(0) || refs["maxFirstFrames"] != one || refs["maxLastFrames"] != int64(0) || slices.Contains(projected["operations"].([]string), "text_to_video") || projected["defaultResolution"] != "720p" || len(projected["ratios"].([]any)) != 1 || !projected["durationSupported"].(bool) || !projected["generateAudio"].(map[string]any)["supported"].(bool) {
		t.Fatalf("incorrect reference projection: %+v", projected)
	}
	selected.NativeRequest.References[0].MaxItems = nil
	refs = studioCapabilityProjection(selected)["video"].(map[string]any)["references"].(map[string]any)
	if _, present := refs["maxFirstFrames"]; present {
		t.Fatalf("invented cap for unbounded slot: %+v", refs)
	}
	selected.Kind = "image"
	selected.Schema = json.RawMessage(`{"properties":{"/generationConfig/imageConfig/imageSize":{"enum":["1K","2K"]},"/generationConfig/imageConfig/aspectRatio":{"enum":["1:1"]},"/quality":{"enum":["high"]},"/background":{"enum":["opaque","transparent"]}}}`)
	image := studioCapabilityProjection(selected)["image"].(map[string]any)
	if len(image["resolution"].(map[string]any)["values"].([]any)) != 2 || image["size"].(map[string]any)["parameter"] != "aspect_ratio" || image["quality"].(map[string]any)["supported"] != true || image["transparentBackground"].(map[string]any)["supported"] != true {
		t.Fatalf("image controls lost their native mappings: %+v", image)
	}
}
