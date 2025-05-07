// Package model internal/model/pnl.go
package model

// PnLResult holds profit‑and‑loss data for any DEX.
type PnLResult struct {
	InitialInvestment float64 // invested amount
	SellEstimate      float64 // value if sold now (fee‑adjusted)
	NetPnL            float64 // profit / loss
	PnLPercentage     float64 // NetPnL ÷ InitialInvestment x 100
}
