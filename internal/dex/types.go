// ==========================================
// File: internal/dex/types.go (modified)
// ==========================================
package dex

// OperationType defines a DEX operation type.
type OperationType string

const (
	OperationSnipe OperationType = "snipe"
	OperationSell  OperationType = "sell"
	OperationSwap  OperationType = "swap"
)

// Task represents an operation request for DEX.
type Task struct {
	Operation    OperationType
	Amount       uint64
	MinSolOutput uint64
	TokenMint    string // Added TokenMint to identify token
}
