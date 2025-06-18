package monitor

import "time"

// Position represents a trading position
type Position struct {
	SessionID   string
	WalletAddr  string
	TokenMint   string
	TokenSymbol string
	Amount      float64
	InitialSOL  float64
	CurrentSOL  float64
	PnL         float64
	PnLPercent  float64
	Status      string
	UpdatedAt   time.Time
}
