// =============================================
// File: internal/task/task.go
// =============================================
package task

import (
	"time"
)

// OperationType is the kind of trading action to perform.
type OperationType string

const (
	OperationSnipe OperationType = "snipe"
	OperationSwap  OperationType = "swap"
	OperationSell  OperationType = "sell"
)

// Task holds parameters for a trade operation loaded from CSV.
type Task struct {
	ID              int           // Unique row index
	TaskName        string        // Identifier or name
	Module          string        // Module name (for routing)
	WalletName      string        // Name of the wallet config
	Operation       OperationType // Type of operation to execute
	AmountSol       float64       // SOL amount to spend or tokens amount to sell
	SlippagePercent float64       // Allowed slippage percent
	PriorityFeeSol  string        // Priority fee, e.g. "0.000001" or "default"
	ComputeUnits    uint32        // Compute units for transaction
	TokenMint       string        // Token mint address
	CreatedAt       time.Time     // Timestamp when task was parsed
	AutosellAmount  float64       // Percent of tokens to auto-sell
}
