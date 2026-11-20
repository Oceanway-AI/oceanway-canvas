package model

// StudioTaskFunds projects the original Platform receipt. It is never persisted
// as a Studio account balance or local billing order.
type StudioTaskFunds struct {
	FundsState         string                    `json:"fundsState"`
	SettledPoints      string                    `json:"settledPoints,omitempty"`
	StateAvailable     bool                      `json:"stateAvailable"`
	Summary            string                    `json:"summary"`
	FinalWalletConsent *StudioFinalWalletConsent `json:"finalWalletConsent,omitempty"`
}

type StudioFinalWalletConsent struct {
	State       string `json:"state"`
	TotalPoints string `json:"totalPoints"`
}
