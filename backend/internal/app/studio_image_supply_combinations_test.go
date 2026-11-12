package app

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"yingce/backend/internal/platformintegration"
)

// This is a consumer constraint regression, not a new manufacturer capability.
// Copy the actual Gateway -> Platform projection in memory, then narrow two
// complete supply branches to distinct quality/size combinations. All four
// scalar combinations fit the published envelope; only two have one supplier.
func TestStudioImageSupplyCombinationsRejectCrossBranchMix(t *testing.T) {
	raw, err := os.ReadFile("../platformintegration/testdata/image-output-size/gpt-4mp.model.json")
	if err != nil {
		t.Fatal(err)
	}
	var catalogModel platformclient.Model
	if err := json.Unmarshal(raw, &catalogModel); err != nil {
		t.Fatal(err)
	}
	selected, err := platformintegration.SelectModel(catalogModel, "")
	if err != nil {
		t.Fatal(err)
	}
	variant := selected.NativeRequest
	var schema map[string]any
	if err := json.Unmarshal(variant.Schema, &schema); err != nil {
		t.Fatal(err)
	}
	var original []map[string]any
	if err := json.Unmarshal(variant.SupplyAlternatives, &original); err != nil || len(original) != 1 {
		t.Fatalf("expected the actual one-supply projection: %v", err)
	}
	originalSupply, err := json.Marshal(original[0])
	if err != nil {
		t.Fatal(err)
	}
	conditions := []struct{ quality, size string }{{"low", "1024x1024"}, {"high", "2048x2048"}}
	branches, alternatives := []any{}, []map[string]any{}
	for _, condition := range conditions {
		var branch, alternative map[string]any
		if err := json.Unmarshal(variant.Schema, &branch); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(originalSupply, &alternative); err != nil {
			t.Fatal(err)
		}
		branch["allOf"] = append(branch["allOf"].([]any), map[string]any{
			"properties": map[string]any{
				"size":    map[string]any{"const": condition.size},
				"quality": map[string]any{"const": condition.quality},
			},
			"required": []any{"size", "quality"},
		})
		alternative["request_schema"] = branch
		branches = append(branches, branch)
		alternatives = append(alternatives, alternative)
	}
	schema["allOf"] = append(schema["allOf"].([]any), map[string]any{"anyOf": branches})
	variant.Schema, err = json.Marshal(schema)
	if err != nil {
		t.Fatal(err)
	}
	variant.SupplyAlternatives, err = json.Marshal(alternatives)
	if err != nil {
		t.Fatal(err)
	}
	variantJSON, err := json.Marshal(variant)
	if err != nil {
		t.Fatal(err)
	}
	before, err := json.Marshal(selected)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		quality, size string
		allowed       bool
	}{
		{"low", "1024x1024", true},
		{"high", "2048x2048", true},
		{"low", "2048x2048", false},
		{"high", "1024x1024", false},
	} {
		t.Run(tc.quality+"/"+tc.size, func(t *testing.T) {
			parameters := map[string]any{"/quality": tc.quality, "/size": tc.size}
			req := CreateTaskRequest{Prompt: "A still life", Input: map[string]any{"config": map[string]any{
				"channelId": StudioPlatformChannelID, "model": selected.Name,
				"nativeVariantId": selected.ID, "nativeParameters": parameters,
			}}}
			input, err := studioDecodeInput(req)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := studioNativeSpecifications(input, selected); err != nil {
				t.Fatalf("all scalar values must remain legal in the union: %v", err)
			}
			candidate, _ := json.Marshal(map[string]any{"size": tc.size, "quality": tc.quality})
			if err := platformclient.ValidateNativeImageSize(variantJSON, candidate); err != nil {
				t.Fatalf("the size envelope alone must not decide supplier combinations: %v", err)
			}
			body, err := (&Service{}).studioMediaBody(t.Context(), "user", req, selected)
			if (err == nil) != tc.allowed {
				t.Fatalf("allowed=%v, want %v: %v", err == nil, tc.allowed, err)
			}
			if !tc.allowed {
				if body != nil {
					t.Fatal("an unsupported combination must not produce a preview/submit body")
				}
				return
			}
			var prepared map[string]any
			if err := json.Unmarshal(body, &prepared); err != nil {
				t.Fatal(err)
			}
			if prepared["quality"] != tc.quality || prepared["size"] != tc.size {
				t.Fatal("a supported supply combination was silently rewritten")
			}
		})
	}
	after, err := json.Marshal(selected)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("request validation mutated the published supplier alternatives")
	}
}
