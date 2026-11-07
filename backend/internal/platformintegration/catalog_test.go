package platformintegration

import (
	"encoding/json"
	"testing"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
)

func TestPublishedMediaVariantDoesNotUnionAlternativeParameters(t *testing.T) {
	m := platformclient.Model{ModelName: "exact-image", ModelType: "image", InvocationStatus: "available", Price: json.RawMessage(`{"status":"usage_dependent","version":"v1"}`), MediaGenerationProfile: json.RawMessage(`{"mode":"image-openai","default_variant":"generate","variants":[{"id":"generate","protocol":"openai-images","operation":"generate","method":"POST","path":"/images/generations","defaults":{"size":"1024x1024","n":1},"specifications_schema":{"type":"object","properties":{"size":{"enum":["1024x1024"]}}},"parameter_bindings":{"size":"size"}},{"id":"edit","protocol":"openai-images","operation":"edit","method":"POST","path":"/images/edits","defaults":{"size":"auto"},"specifications_schema":{"type":"object","properties":{"size":{"enum":["auto"]}}}}]}`)}
	selected, err := SelectModel(m, "")
	if err != nil {
		t.Fatal(err)
	}
	if selected.Path != "/images/generations" || selected.Defaults["size"] != "1024x1024" {
		t.Fatalf("wrong default variant: %+v", selected)
	}
	if string(selected.Schema) == "" || string(selected.Schema) == "null" {
		t.Fatal("missing schema")
	}
	edit, err := SelectModel(m, "edit")
	if err != nil || edit.Path != "/images/edits" || edit.Defaults["size"] != "auto" {
		t.Fatalf("edit was merged: %+v %v", edit, err)
	}
	if _, err := SelectModel(m, "missing"); err == nil {
		t.Fatal("unknown variant accepted")
	}
	m.InvocationStatus = "price_only"
	if _, err := SelectModel(m, ""); err == nil {
		t.Fatal("price-only model accepted")
	}
}

func TestPublishedTextRequiresDeclaredProtocolAndPriceVersion(t *testing.T) {
	m := platformclient.Model{ModelName: "text", ModelType: "text", InvocationStatus: "published_text", SupportedProtocols: []string{"openai-chat"}, Price: json.RawMessage(`{"status":"available","version":"v1"}`)}
	selected, err := SelectModel(m, "")
	if err != nil || selected.TextProtocol != platformclient.ChatCompletions {
		t.Fatalf("text selection: %+v %v", selected, err)
	}
	m.SupportedProtocols = []string{"unknown"}
	if _, err = SelectModel(m, ""); err == nil {
		t.Fatal("unknown text transport inferred from model name")
	}
	m.SupportedProtocols = []string{"openai-chat"}
	m.Price = json.RawMessage(`{"status":"unavailable"}`)
	if _, err = SelectModel(m, ""); err == nil {
		t.Fatal("unavailable pricing admitted")
	}
}
