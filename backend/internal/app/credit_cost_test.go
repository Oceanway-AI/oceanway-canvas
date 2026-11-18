package app

import (
	"testing"

	"yingce/backend/internal/model"
)

func TestBillingCreditCostUsesExactUsageAndIgnoresSalesMultiplier(t *testing.T) {
	for _, tc := range []struct {
		name, mode, capability                         string
		quantity, input, output, cached, formula, want int64
		usage                                          bool
	}{
		{name: "request", mode: "fixed_request", quantity: 1, want: 90_000},
		{name: "seconds", mode: "per_second", quantity: 15, want: 1_350_000},
		{name: "text cached tokens", mode: "token", capability: "text", input: 1_000_000, output: 500_000, cached: 200_000, usage: true, want: 1_820_000},
		{name: "video formula", mode: "token", capability: "video", formula: 250_000, want: 500_000},
		{name: "video provider", mode: "token", capability: "video", output: 100_000, formula: 250_000, usage: true, want: 200_000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			order := model.BillingOrder{Status: model.BillingStatusSettled, Capability: tc.capability, InputTokens: tc.input, OutputTokens: tc.output, CachedTokens: tc.cached, UsageAvailable: tc.usage, MultiplierBasisPoints: 30_000, ChargeLimitMicrocredits: 1,
				BillingCostSnapshot: model.BillingCostSnapshot{CostBillingMode: tc.mode, CostQuantity: tc.quantity, CostVideoFormulaTokens: tc.formula,
					CostPricing: model.CreditCostPricing{Configured: true, UnitPriceMicrocredits: 90_000, InputTokenPriceMicrocredits: 1_000_000, OutputTokenPriceMicrocredits: 2_000_000, CachedTokenPriceMicrocredits: 100_000}}}
			got, err := billingCreditCost(order)
			if err != nil || got == nil || *got != tc.want {
				t.Fatalf("cost=%v want=%d err=%v", got, tc.want, err)
			}
			order.CostPricing = model.CreditCostPricing{Configured: true}
			got, err = billingCreditCost(order)
			if err != nil || got == nil || *got != 0 {
				t.Fatal("explicit zero cost must remain available")
			}
			for _, status := range []model.BillingStatus{model.BillingStatusReserved, model.BillingStatusRunning, model.BillingStatusRefunded, model.BillingStatusUncertain} {
				order.Status = status
				got, err = billingCreditCost(order)
				if err != nil || got != nil {
					t.Fatalf("unsettled cost fabricated for %s", status)
				}
			}
		})
	}
}

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
