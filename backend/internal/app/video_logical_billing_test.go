package app

import (
	"encoding/json"
	"testing"

	"yingce/backend/internal/model"
)

func TestUnifiedVideoQuoteAndTaskUseResolvedProviderGeneration(t *testing.T) {
	svc, db, _, channelModel, profile := newVideoTokenQuoteFixture(t)
	const upstream = "doubao-seedance-2-5"
	channelModel.ProviderModelKey = upstream
	if err := db.Model(&model.ChannelModel{}).Where("id = ?", channelModel.ID).Update("provider_model_key", upstream).Error; err != nil {
		t.Fatal(err)
	}
	spec, err := CapabilitySpecFromModelCapabilityConfig(profile, "video")
	if err != nil {
		t.Fatal(err)
	}
	specJSON, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	logical := model.LogicalModel{
		ID: "logical-aliased-video", Code: "video-alias", Capability: "video", Enabled: true,
		ActiveRevisionID: "aliased-video-revision", PricePolicy: "unified", BillingMode: "token", OutputPriceMicrocredits: 16_000_000,
	}
	revision := model.LogicalModelRevision{
		ID: logical.ActiveRevisionID, LogicalModelID: logical.ID, Version: 1, CapabilitySpecJSON: string(specJSON),
		DefaultOptionsJSON: `{"vquality":"480p","size":"16:9","videoSeconds":5,"videoGenerateAudio":true}`,
	}
	route := model.LogicalModelRoute{ID: "aliased-video-route", LogicalModelRevisionID: revision.ID, ChannelModelID: channelModel.ID, Enabled: true, Weight: 1}
	for _, row := range []any{&logical, &revision, &route} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	quote, err := svc.QuoteLogicalModel(testGroupUserID(svc), logical.ID, ModelRequestIntent{Capability: "video"})
	if err != nil {
		t.Fatal(err)
	}
	routed, input, err := svc.resolveTaskModelSelection(testGroupUserID(svc), map[string]any{"mode": "video", "config": map[string]any{}}, logical.ID, "canvas_video", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if routed.PriceTier != nil || input["config"].(map[string]any)["providerModelKey"] != upstream {
		t.Fatalf("unified route did not preserve its provider model: %#v", input)
	}
	order, err := svc.taskBillingOrder("quote-user", &model.Task{
		ID: "aliased-video-task", Type: "canvas_video", LogicalModelID: logical.ID,
		LogicalModelRevisionID: revision.ID, RouteID: route.ID, ChannelModelID: channelModel.ID,
	}, input)
	if err != nil {
		t.Fatal(err)
	}
	if quote.VideoTokenEstimate == nil || quote.VideoTokenEstimate.OutputWidth != 854 || quote.VideoTokenEstimate.OutputHeight != 480 || quote.VideoTokenEstimate.DimensionsEstimated {
		t.Fatalf("wrong generation-specific quote: %#v", quote.VideoTokenEstimate)
	}
	if order == nil || order.VideoFormulaTokens != 48038 || order.Quantity != 52842 || order.AmountMicrocredits != 845472 || quote.Quantity != order.Quantity || quote.AmountMicrocredits != order.AmountMicrocredits {
		t.Fatalf("quote/task mismatch: quote=%#v order=%#v", quote, order)
	}
	assertVideoQuoteHasNoFinancialWrites(t, db)
}

func TestRoutedProviderSelectionFallsBackWithoutTierOverride(t *testing.T) {
	for _, test := range []struct {
		name     string
		provider string
		tier     *model.ChannelModelPriceTier
		want     string
	}{
		{name: "unified without tier", provider: "provider-default", want: "provider-default"},
		{name: "empty tier override", provider: "provider-default", tier: &model.ChannelModelPriceTier{ID: "tier"}, want: "provider-default"},
		{name: "tier override", provider: "provider-default", tier: &model.ChannelModelPriceTier{ID: "tier", ProviderModelKey: "provider-tier"}, want: "provider-tier"},
		{name: "model key fallback", tier: &model.ChannelModelPriceTier{ID: "tier"}, want: "model-alias"},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := applyRoutedProviderSelection(map[string]any{"config": map[string]any{"providerModelKey": "untrusted-client-model"}}, &RoutedModel{
				ChannelModel: model.ChannelModel{ChannelID: "channel", ModelKey: "model-alias", ProviderModelKey: test.provider}, PriceTier: test.tier,
			})
			if got := input["config"].(map[string]any)["providerModelKey"]; got != test.want {
				t.Fatalf("providerModelKey = %v, want %s", got, test.want)
			}
		})
	}
}
