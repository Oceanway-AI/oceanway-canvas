package handler

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestStudioCatalogPresentationHTTP(t *testing.T) {
	f := newStudioFixture(t)
	f.catalog = `{"success":true,"data":{"items":[{"id":41,"model_name":"exact-text","model_series":"GPT","logical_model":"Text","model_type":"text","invocation_status":"published_text","supported_protocols":["openai-chat"],"price":{"status":"available","version":"p1","rates":{"input":"0.00000000123456789"}},"content":{"display_name":"文本型号","tagline":"准确说明","tags":["推理"]},"presentation":{"version":"model-presentation.v1","summary":"准确说明","tags":["推理"],"features":[],"availability":{"state":"available"},"price_summary":[],"health":{"scope":"platform","basis":"generation_outcome","state":"no_data","success_rate":null,"buckets":[]}},"media_generation_profile":{"extended":{"precise":9007199254740993}}}],"groups":[{"key":"text-gpt","model_series":"GPT","model_type":"text","logical_model":"","display_name":"GPT","model_names":["exact-text"],"available_count":1,"health":{"scope":"platform","basis":"generation_outcome","state":"no_data","success_rate":null,"buckets":[]},"price_summary":[]}]}}`
	if path := os.Getenv("STUDIO_PRESENTATION_PROVIDER_FIXTURE"); path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		f.catalog = string(raw)
	}
	w := f.request(t, "GET", "/api/model-catalog", "platform_session=alice", "", 200)
	var result struct {
		Data struct {
			Channels []struct {
				PlatformGroups []json.RawMessage `json:"platformGroups"`
				Models         []struct {
					Platform  json.RawMessage `json:"platform"`
					Available bool            `json:"available"`
				} `json:"models"`
			} `json:"channels"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Data.Channels) != 1 || len(result.Data.Channels[0].PlatformGroups) == 0 {
		t.Fatal("Platform groups lost at host HTTP boundary")
	}
	for _, model := range result.Data.Channels[0].Models {
		if !model.Available {
			t.Fatal("published fixture model is not selectable through the actual SDK")
		}
		if !strings.Contains(string(model.Platform), `"presentation"`) {
			t.Fatal("presentation lost at host HTTP boundary")
		}
	}
	if os.Getenv("STUDIO_PRESENTATION_PROVIDER_FIXTURE") == "" {
		if !result.Data.Channels[0].Models[0].Available || !strings.Contains(w.Body.String(), "9007199254740993") || !strings.Contains(w.Body.String(), "0.00000000123456789") {
			t.Fatal("typed execution or lossless public presentation changed")
		}
	}
	if path := os.Getenv("STUDIO_PRESENTATION_HOST_FIXTURE"); path != "" {
		if err := os.WriteFile(path, w.Body.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if f.catalogCalls != 1 {
		t.Fatal("catalog did not use SDK once")
	}
	f.request(t, "GET", "/api/model-catalog", "", "", 401)
	if f.catalogCalls != 1 {
		t.Fatal("anonymous catalog reached provider")
	}
}
