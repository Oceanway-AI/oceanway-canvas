package app

import (
	"testing"

	"yingce/backend/internal/model"
)

func TestCreditCostRejectsInvalidPrices(t *testing.T) {
	for _, price := range []int64{-1, 1_000_000_000_001} {
		if err := validateCreditCostPricing("text", "fixed_request", model.CreditCostPricing{Configured: true, UnitPriceMicrocredits: price}); err == nil {
			t.Fatalf("accepted cost %d", price)
		}
	}
	if err := validateCreditCostPricing("video", "token", model.CreditCostPricing{Configured: true, InputTokenPriceMicrocredits: 1}); err == nil {
		t.Fatal("accepted video input cost")
	}
}
