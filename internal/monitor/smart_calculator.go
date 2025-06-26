// internal/monitor/smart_calculator.go
package monitor

import (
	"context"
	"fmt"
	"github.com/rovshanmuradov/solana-bot/internal/dex"
	"github.com/rovshanmuradov/solana-bot/internal/dex/model"
	"go.uber.org/zap"
)

// smartDEXCalculator реализует расчет PnL для Smart DEX адаптера
type smartDEXCalculator struct {
	dex    dex.DEX
	logger *zap.Logger
}

// CalculatePnL вычисляет PnL, используя внутренний адаптер
func (c *smartDEXCalculator) CalculatePnL(ctx context.Context, tokenAmount float64, initialInvestment float64) (*model.PnLResult, error) {
	// Просто делегируем расчет к DEX адаптеру, который сам выберет правильный подадаптер
	pnl, err := c.dex.CalculatePnL(ctx, tokenAmount, initialInvestment)
	if err != nil {
		c.logger.Error("smart_dex.CalculatePnL error", zap.Error(err))
		return nil, fmt.Errorf("failed to calculate PnL: %w", err)
	}
	return pnl, nil
}
