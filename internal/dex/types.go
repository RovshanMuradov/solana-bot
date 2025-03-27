// ==========================================
// File: internal/dex/types.go (modified)
// ==========================================
package dex

import "time"

// OperationType defines a DEX operation type.
type OperationType string

const (
	OperationSnipe OperationType = "snipe"
	OperationSell  OperationType = "sell"
	OperationSwap  OperationType = "swap"
)

// Task represents an operation request for DEX.
type Task struct {
	Operation       OperationType
	AmountSol       float64       // Amount in SOL the user wants to spend (buy) or sell
	SlippagePercent float64       // Slippage tolerance in percent (0-100)
	TokenMint       string        // Token mint address
	PriorityFee     string        // Priority fee in SOL (string format, e.g. "0.000001")
	ComputeUnits    uint32        // Compute units for the transaction
	MonitorInterval time.Duration // Интервал обновления цены при мониторинге (новое поле)
}

// SwapParams содержит параметры для выполнения свапа
type SwapParams struct {
	IsBuy           bool
	Amount          uint64
	SlippagePercent float64
	PriorityFeeSol  string
	ComputeUnits    uint32
}
