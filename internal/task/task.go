// =============================================
// File: internal/task/task.go
// =============================================
package task

import (
	"fmt"
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

// Validate checks if the task has valid parameters
func (t *Task) Validate() error {
	if t.TaskName == "" {
		return fmt.Errorf("task name cannot be empty")
	}

	if t.Module == "" {
		return fmt.Errorf("module cannot be empty")
	}

	if t.WalletName == "" {
		return fmt.Errorf("wallet name cannot be empty")
	}

	if t.TokenMint == "" {
		return fmt.Errorf("token mint cannot be empty")
	}

	// Validate operation
	switch t.Operation {
	case OperationSnipe, OperationSell, OperationSwap:
		// Valid operations
	default:
		return fmt.Errorf("invalid operation: %s", t.Operation)
	}

	// Validate numeric values
	if t.AmountSol <= 0 {
		return fmt.Errorf("amount must be greater than zero")
	}

	const MinSlippage = 0.1
	if t.SlippagePercent < MinSlippage || t.SlippagePercent > 100 {
		return fmt.Errorf("slippage must be between %.1f and 100", MinSlippage)
	}

	return nil
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
