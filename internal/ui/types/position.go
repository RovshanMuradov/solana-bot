package types

import "time"

// MonitoredPosition represents a trading position being monitored
type MonitoredPosition struct {
	ID           int
	TaskName     string
	TokenMint    string
	TokenSymbol  string
	EntryPrice   float64
	CurrentPrice float64
	Amount       float64
	PnLPercent   float64
	PnLSol       float64
	Volume24h    float64
	LastUpdate   time.Time
	PriceHistory []float64
	Active       bool
}
