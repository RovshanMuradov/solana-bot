// =============================
// File: internal/dex/types.go
// =============================

package dex

import (
	"context"
	"github.com/rovshanmuradov/solana-bot/internal/dex/pumpswap"
	"time"
)

const (
	OperationSnipe OperationType = "snipe"
	OperationSell  OperationType = "sell"
	OperationSwap  OperationType = "swap"
)

// OperationType defines a DEX operation type.
type OperationType string

// Task represents an operation request for DEX.
type Task struct {
	Operation       OperationType
	AmountSol       float64       // Amount in SOL the user wants to spend (buy) or sell
	SlippagePercent float64       // Slippage tolerance in percent (0-100)
	TokenMint       string        // Token mint address
	PriorityFee     string        // Priority fee in SOL (string format, e.g. "0.000001")
	ComputeUnits    uint32        // Compute units for the transaction
	MonitorInterval time.Duration // Интервал обновления цены при мониторинге
}

// pumpswapDEXAdapter адаптирует Pump.swap к интерфейсу DEX
type pumpswapDEXAdapter struct {
	baseDEXAdapter
	inner *pumpswap.DEX
}

// DEX — единый интерфейс для работы с различными DEX.
type DEX interface {
	// GetName возвращает название биржи.
	GetName() string
	// Execute выполняет операцию, описанную в задаче.
	Execute(ctx context.Context, task *Task) error
	// GetTokenPrice возвращает текущую цену токена
	GetTokenPrice(ctx context.Context, tokenMint string) (float64, error)
}
