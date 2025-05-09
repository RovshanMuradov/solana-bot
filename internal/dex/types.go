// =============================
// File: internal/dex/types.go
// =============================

package dex

import (
	"context"
	"github.com/rovshanmuradov/solana-bot/internal/dex/model"
	"github.com/rovshanmuradov/solana-bot/internal/task"
)

// DEX — единый интерфейс для работы с различными DEX.
type DEX interface {
	// GetName возвращает название биржи.
	GetName() string
	// Execute выполняет операцию, описанную в задаче.
	Execute(ctx context.Context, task *task.Task) error
	// GetTokenPrice возвращает текущую цену токена
	GetTokenPrice(ctx context.Context, tokenMint string) (float64, error)
	// GetTokenBalance возвращает текущий баланс токена в кошельке пользователя
	GetTokenBalance(ctx context.Context, tokenMint string) (uint64, error)
	// SellPercentTokens продает указанный процент имеющихся токенов
	SellPercentTokens(ctx context.Context, tokenMint string, percentToSell float64, slippagePercent float64, priorityFeeSol string, computeUnits uint32) error
	CalculatePnL(ctx context.Context, tokenAmount float64, initialInvestment float64) (*model.PnLResult, error)
}
