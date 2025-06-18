package monitor

import (
	"fmt"
	"time"
)

// Trade represents a completed trade with all relevant details
type Trade struct {
	ID          string    `json:"id"`
	Timestamp   time.Time `json:"timestamp"`
	WalletAddr  string    `json:"wallet_addr"`
	TokenMint   string    `json:"token_mint"`
	TokenSymbol string    `json:"token_symbol"`
	Action      string    `json:"action"` // "buy" or "sell"
	AmountSOL   float64   `json:"amount_sol"`
	AmountToken float64   `json:"amount_token"`
	Price       float64   `json:"price"`
	TxSignature string    `json:"tx_signature"`

	// For sells only
	EntryPrice float64 `json:"entry_price,omitempty"`
	ExitPrice  float64 `json:"exit_price,omitempty"`
	PnL        float64 `json:"pnl,omitempty"`
	PnLPercent float64 `json:"pnl_percent,omitempty"`
	HoldTime   string  `json:"hold_time,omitempty"`

	// Additional metadata
	DEX         string `json:"dex,omitempty"`
	SlippageBPS int    `json:"slippage_bps,omitempty"`
	GasUsed     uint64 `json:"gas_used,omitempty"`
	Success     bool   `json:"success"`
	ErrorMsg    string `json:"error_msg,omitempty"`
}

// ToCSV converts trade to CSV record
func (t *Trade) ToCSV() []string {
	return []string{
		t.ID,
		t.Timestamp.Format(time.RFC3339),
		t.WalletAddr,
		t.TokenMint,
		t.TokenSymbol,
		t.Action,
		formatFloat(t.AmountSOL),
		formatFloat(t.AmountToken),
		formatFloat(t.Price),
		t.TxSignature,
		formatFloat(t.EntryPrice),
		formatFloat(t.ExitPrice),
		formatFloat(t.PnL),
		formatFloat(t.PnLPercent),
		t.HoldTime,
		t.DEX,
		formatInt(t.SlippageBPS),
		formatUint64(t.GasUsed),
		formatBool(t.Success),
		t.ErrorMsg,
	}
}

// CSVHeaders returns the header row for trade CSV files
func CSVHeaders() []string {
	return []string{
		"id",
		"timestamp",
		"wallet_addr",
		"token_mint",
		"token_symbol",
		"action",
		"amount_sol",
		"amount_token",
		"price",
		"tx_signature",
		"entry_price",
		"exit_price",
		"pnl",
		"pnl_percent",
		"hold_time",
		"dex",
		"slippage_bps",
		"gas_used",
		"success",
		"error_msg",
	}
}

// Helper functions for formatting
func formatFloat(f float64) string {
	if f == 0 {
		return ""
	}
	return fmt.Sprintf("%.6f", f)
}

func formatInt(i int) string {
	if i == 0 {
		return ""
	}
	return fmt.Sprintf("%d", i)
}

func formatUint64(u uint64) string {
	if u == 0 {
		return ""
	}
	return fmt.Sprintf("%d", u)
}

func formatBool(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// CalculateHoldTime calculates the duration between buy and sell
func CalculateHoldTime(buyTime, sellTime time.Time) string {
	duration := sellTime.Sub(buyTime)
	if duration < time.Minute {
		return fmt.Sprintf("%ds", int(duration.Seconds()))
	} else if duration < time.Hour {
		return fmt.Sprintf("%dm", int(duration.Minutes()))
	} else if duration < 24*time.Hour {
		return fmt.Sprintf("%dh%dm", int(duration.Hours()), int(duration.Minutes())%60)
	}
	days := int(duration.Hours() / 24)
	hours := int(duration.Hours()) % 24
	return fmt.Sprintf("%dd%dh", days, hours)
}
