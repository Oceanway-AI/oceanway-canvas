package app

import (
	"encoding/json"

	"yingce/backend/internal/model"
)

func studioTaskFunds(task model.Task) *model.StudioTaskFunds {
	if task.Provider != "platform" {
		return nil
	}
	funds := &model.StudioTaskFunds{FundsState: "unknown", Summary: "当前任务尚无平台结算回执，费用未知"}
	var result struct {
		FundsState    string `json:"fundsState"`
		SettledPoints string `json:"settledPoints"`
	}
	if json.Unmarshal([]byte(task.ResultJSON), &result) != nil || result.FundsState == "" {
		if task.Type == "canvas_text" {
			return nil
		}
		return funds
	}
	funds.FundsState, funds.StateAvailable = result.FundsState, true
	funds.Summary = "平台正在处理本任务资金，实际费用以结算回执为准"
	if result.FundsState == "settled" && studioValidDecimal(result.SettledPoints) {
		funds.SettledPoints = result.SettledPoints
		funds.Summary = "本任务已结算，settledPoints 为实际平台积分"
	}
	if result.FundsState == "released" {
		funds.Summary = "平台已释放本任务预留资金"
	}
	return funds
}
