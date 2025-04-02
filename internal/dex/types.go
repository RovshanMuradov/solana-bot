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
	// GetTokenBalance возвращает текущий баланс токена в кошельке пользователя
	GetTokenBalance(ctx context.Context, tokenMint string) (uint64, error)
	// SellPercentTokens продает указанный процент имеющихся токенов
	SellPercentTokens(ctx context.Context, tokenMint string, percentToSell float64, slippagePercent float64, priorityFeeSol string, computeUnits uint32) error
	// CalculateDiscretePnL вычисляет PnL с учетом дискретной структуры Pump.fun
	CalculateDiscretePnL(ctx context.Context, tokenAmount float64, initialInvestment float64) (*DiscreteTokenPnL, error)
}

// DiscreteTokenPnL содержит информацию о PnL с учетом дискретной природы токена
type DiscreteTokenPnL struct {
	CurrentPrice      float64 // Текущая цена токена
	TheoreticalValue  float64 // Теоретическая стоимость (цена * количество)
	SellEstimate      float64 // Оценка реальной выручки при продаже
	InitialInvestment float64 // Начальная инвестиция
	NetPnL            float64 // Чистый PnL (SellEstimate - InitialInvestment)
	PnLPercentage     float64 // Процент PnL
}
