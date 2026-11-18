package app

import (
	"yingce/backend/internal/model"
)

func validateCreditCostPricing(capability, billingMode string, cost model.CreditCostPricing) error {
	for _, price := range []int64{cost.UnitPriceMicrocredits, cost.InputTokenPriceMicrocredits, cost.OutputTokenPriceMicrocredits, cost.CachedTokenPriceMicrocredits, cost.CacheWriteTokenPriceMicrocredits, cost.ReferenceVideoUnitPriceMicrocredits, cost.ReferenceVideoOutputTokenPriceMicrocredits} {
		if price < 0 || price > 1_000_000_000_000 {
			return BadAuthRequest("积分成本价必须在 0 到 1000000 积分之间")
		}
	}
	if !cost.Configured {
		return nil
	}
	if billingMode != "token" {
		capability = ""
	}
	return validateTokenPrices(capability, cost.InputTokenPriceMicrocredits, cost.OutputTokenPriceMicrocredits, cost.CachedTokenPriceMicrocredits, cost.CacheWriteTokenPriceMicrocredits)
}
