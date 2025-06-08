package task

import (
	"fmt"
	"time"
)

// OperationType is the kind of trading action to perform.
type OperationType string

const (
	OperationSnipe OperationType = "snipe"
	OperationSwap  OperationType = "swap"
	OperationSell  OperationType = "sell"
)

// IsValid checks if the operation type is valid.
func (op OperationType) IsValid() bool {
	switch op {
	case OperationSnipe, OperationSwap, OperationSell:
		return true
	default:
		return false
	}
}

// String implements the Stringer interface.
func (op OperationType) String() string {
	return string(op)
}

// Task holds parameters for a trade operation loaded from YAML.
type Task struct {
	ID              int           // Unique task index
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

	// New fields for protocol support
	Protocol  string                 // Protocol type: "dex", "nft", "staking"
	AssetType string                 // Asset type: "token", "nft", "lp"
	Metadata  map[string]interface{} // Extensible metadata for future use
}

// Validate checks if the task has all required fields and valid values.
func (t *Task) Validate() error {
	// Check required fields
	if t.TaskName == "" {
		return fmt.Errorf("task name is required")
	}
	if t.Module == "" {
		return fmt.Errorf("module is required")
	}
	if t.WalletName == "" {
		return fmt.Errorf("wallet name is required")
	}
	if t.TokenMint == "" {
		return fmt.Errorf("token mint is required")
	}

	// Validate operation
	if !t.Operation.IsValid() {
		return fmt.Errorf("invalid operation: %s", t.Operation)
	}

	// Validate amount for buy operations
	if (t.Operation == OperationSnipe || t.Operation == OperationSwap) && t.AmountSol <= 0 {
		return fmt.Errorf("amount must be greater than 0 for %s operation", t.Operation)
	}

	// Validate slippage
	if t.SlippagePercent < 0 || t.SlippagePercent > 100 {
		return fmt.Errorf("slippage percent must be between 0 and 100")
	}

	// Validate autosell amount
	if t.AutosellAmount < 0 || t.AutosellAmount > 100 {
		return fmt.Errorf("autosell amount must be between 0 and 100")
	}

	return nil
}

// IsBuyOperation returns true if the task is a buy operation (snipe or swap).
func (t *Task) IsBuyOperation() bool {
	return t.Operation == OperationSnipe || t.Operation == OperationSwap
}

// GetProtocol returns the protocol type, defaulting to "dex" for backward compatibility.
func (t *Task) GetProtocol() string {
	if t.Protocol == "" {
		return "dex" // Default for backward compatibility
	}
	return t.Protocol
}

// GetAssetType returns the asset type, defaulting to "token" for backward compatibility.
func (t *Task) GetAssetType() string {
	if t.AssetType == "" {
		return "token" // Default for backward compatibility
	}
	return t.AssetType
}

// GetMetadata returns the metadata map, initializing it if nil.
func (t *Task) GetMetadata() map[string]interface{} {
	if t.Metadata == nil {
		t.Metadata = make(map[string]interface{})
	}
	return t.Metadata
}
