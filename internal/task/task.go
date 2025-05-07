// =============================================
// File: internal/task/task.go
// =============================================
package task

import (
	"time"

	"github.com/rovshanmuradov/solana-bot/internal/dex"
)

// Constants
const LamportsPerSOL = 1_000_000_000

// OperationType defines the supported operation types
type OperationType string

const (
	OperationSnipe OperationType = "snipe"
	OperationSell  OperationType = "sell"
	OperationSwap  OperationType = "swap"
)

// Task represents a trading task from CSV configuration
type Task struct {
	ID              int
	TaskName        string
	Module          string
	WalletName      string
	Operation       OperationType
	AmountSol       float64 // For buy: Amount in SOL to spend. For sell: Number of tokens to sell
	SlippagePercent float64 // Slippage tolerance in percent (0-100)
	PriorityFeeSol  string  // Priority fee in SOL (string format, e.g. "0.000001")
	ComputeUnits    uint32  // Compute units for the transaction
	TokenMint       string  // Token mint address
	CreatedAt       time.Time
	AutosellAmount  float64
}

// NewTask creates a properly initialized task
func NewTask(name, module, wallet string, op OperationType, amount, slippage float64,
	priority string, compute uint32, tokenMint string) *Task {
	return &Task{
		TaskName:        name,
		Module:          module,
		WalletName:      wallet,
		Operation:       op,
		AmountSol:       amount,
		SlippagePercent: slippage,
		PriorityFeeSol:  priority,
		ComputeUnits:    compute,
		TokenMint:       tokenMint,
		CreatedAt:       time.Now(),
	}
}

// ToDEXTask converts Task to dex.Task format
func (t *Task) ToDEXTask(monitorInterval time.Duration) *dex.Task {
	return &dex.Task{
		Operation:       dex.OperationType(t.Operation),
		AmountSol:       t.AmountSol,
		SlippagePercent: t.SlippagePercent,
		TokenMint:       t.TokenMint,
		PriorityFee:     t.PriorityFeeSol,
		ComputeUnits:    t.ComputeUnits,
		MonitorInterval: monitorInterval,
	}
}
