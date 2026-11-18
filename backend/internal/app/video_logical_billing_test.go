package app

import (
	"testing"

	"yingce/backend/internal/model"
)

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
