package app

import (
	"testing"

	"yingce/backend/internal/model"
)

func TestValidateChannelModelPriceSupportsAllVideoTokens(t *testing.T) {
	const outputPrice = int64(16_000_000)
	if !ValidateChannelModelPrice("token", "video", model.ChannelInterfaceVolcengineArkVideo, 0, 0, outputPrice, 0, 0) {
		t.Fatal("Volcengine Ark video Token price should be valid")
	}
	for _, protocol := range []model.ChannelInterfaceType{model.ChannelInterfaceVolcengineJiMengVideo, model.ChannelInterfaceNewAPIVideo} {
		if !ValidateChannelModelPrice("token", "video", protocol, 0, 0, outputPrice, 0, 0) {
			t.Fatalf("video protocol %q should support Token pricing", protocol)
		}
	}
}

func TestHasValidPriceUsesChannelProtocolForTokenTiers(t *testing.T) {
	tier := model.ChannelModelPriceTier{Enabled: true, PriceConfigured: true, BillingMode: "token", OutputTokenPriceMicrocredits: 16_000_000}
	ark := &model.ChannelModel{Capability: "video", Protocol: model.ChannelInterfaceVolcengineArkVideo, PriceTiers: []model.ChannelModelPriceTier{tier}}
	if !HasValidPrice(ark) {
		t.Fatal("Volcengine Ark video Token tier should be valid")
	}

	jimeng := &model.ChannelModel{Capability: "video", Protocol: model.ChannelInterfaceVolcengineJiMengVideo, PriceTiers: []model.ChannelModelPriceTier{tier}}
	if !HasValidPrice(jimeng) {
		t.Fatal("JiMeng video Token tier should be valid")
	}
}

func TestTokenPricesValidateCacheWritePrice(t *testing.T) {
	if err := validateTokenPrices("text", 1_000_000, 2_000_000, 100_000, 1_250_000); err != nil {
		t.Fatalf("text cache write price rejected: %v", err)
	}
	if err := validateTokenPrices("text", 1, 1, 1, -1); err == nil {
		t.Fatal("negative cache write price accepted")
	}
	if err := validateTokenPrices("video", 0, 1, 0, 1); err == nil {
		t.Fatal("video cache write price accepted")
	}
	if err := validateTokenPrices("text", 1, 1, 1, maxChannelModelTokenPriceMicrocredits+1); err == nil {
		t.Fatal("oversized cache write price accepted")
	}
}
