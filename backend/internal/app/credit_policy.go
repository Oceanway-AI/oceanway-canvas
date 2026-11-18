package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

const creditPolicySettingKey = "credit_policy"

type CreditPolicy struct {
	DefaultMultiplierBPS int64            `json:"defaultMultiplierBasisPoints"`
	ModelMultiplierBPS   map[string]int64 `json:"modelMultiplierBasisPoints"`
}

func defaultCreditPolicy() CreditPolicy {
	return CreditPolicy{DefaultMultiplierBPS: 10_000, ModelMultiplierBPS: map[string]int64{}}
}

func validateCreditPolicy(policy CreditPolicy) error {
	if policy.DefaultMultiplierBPS <= 0 || policy.DefaultMultiplierBPS > 1_000_000 {
		return BadAuthRequest("默认模型倍率必须在 0.0001-100 之间")
	}
	for modelKey, multiplier := range policy.ModelMultiplierBPS {
		if strings.TrimSpace(modelKey) == "" || multiplier <= 0 || multiplier > 1_000_000 {
			return BadAuthRequest("模型倍率配置无效")
		}
	}
	return nil
}

// Load the pricing policy for retained protocol fixtures.
func (s *Service) creditPolicy() (CreditPolicy, error) {
	setting, err := s.repo.SystemSetting(creditPolicySettingKey)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return defaultCreditPolicy(), nil
	}
	if err != nil {
		return CreditPolicy{}, err
	}
	var policy CreditPolicy
	if json.Unmarshal([]byte(setting.ValueJSON), &policy) != nil {
		return CreditPolicy{}, errors.New("积分策略配置格式无效")
	}
	if policy.ModelMultiplierBPS == nil {
		policy.ModelMultiplierBPS = map[string]int64{}
	}
	if err := validateCreditPolicy(policy); err != nil {
		return CreditPolicy{}, err
	}
	return policy, nil
}

// 单价、数量和倍率全程使用整数并向上取整，避免浮点误差造成少扣积分。
func creditAmount(unitPrice int64, quantity int64, multiplierBPS int64) (int64, error) {
	if unitPrice < 0 || quantity <= 0 || multiplierBPS <= 0 {
		return 0, errors.New("积分计费参数无效")
	}
	if unitPrice > (1<<63-1)/quantity {
		return 0, errors.New("积分计费金额溢出")
	}
	base := unitPrice * quantity
	if base > ((1<<63-1)-9_999)/multiplierBPS {
		return 0, errors.New("积分计费金额溢出")
	}
	amount := (base*multiplierBPS + 9_999) / 10_000
	if amount < 0 {
		return 0, fmt.Errorf("积分计费金额无效：%d", amount)
	}
	return amount, nil
}
