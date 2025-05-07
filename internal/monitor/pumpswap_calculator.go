// internal/monitor/pumpswap_calculator.go
package monitor

import (
	"context"
	"fmt"
	"github.com/rovshanmuradov/solana-bot/internal/dex"
	"github.com/rovshanmuradov/solana-bot/internal/dex/model"
	"go.uber.org/zap"
)

// pumpSwapCalculator реализует расчет PnL, специфичный для Pump.swap DEX
type pumpSwapCalculator struct {
	dex    dex.DEX
	logger *zap.Logger
}

// CalculatePnL делегирует расчёт PnL к внутреннему TokenCalculator через интерфейс PnLCalculatorInterface.
func (c *pumpSwapCalculator) CalculatePnL(ctx context.Context, tokenAmount float64, initialInvestment float64) (*model.PnLResult, error) {
	res, err := c.dex.CalculatePnL(ctx, tokenAmount, initialInvestment)
	if err != nil {
		c.logger.Error("dex.CalculatePnL error", zap.Error(err))
		return nil, fmt.Errorf("failed to calculate PnL: %w", err)
	}

	return &model.PnLResult{
		InitialInvestment: res.InitialInvestment,
		SellEstimate:      res.SellEstimate,
		NetPnL:            res.NetPnL,
		PnLPercentage:     res.PnLPercentage,
	}, nil
}
