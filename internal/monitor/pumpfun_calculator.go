// internal/monitor/pumpfun_calculator.go
package monitor

import (
	"context"
	"fmt"
	"github.com/rovshanmuradov/solana-bot/internal/dex"
	"github.com/rovshanmuradov/solana-bot/internal/dex/model"
	"go.uber.org/zap"
)

type pumpFunCalculator struct {
	dex    dex.DEX
	logger *zap.Logger
}

func (c *pumpFunCalculator) CalculatePnL(ctx context.Context, tokenAmount float64, initialInvestment float64) (*model.PnLResult, error) {
	pnl, err := c.dex.CalculatePnL(ctx, tokenAmount, initialInvestment)
	if err != nil {
		c.logger.Error("dex.CalculatePnL error", zap.Error(err))
		return nil, fmt.Errorf("failed to calculate PnL: %w", err)
	}
	return pnl, nil
}
