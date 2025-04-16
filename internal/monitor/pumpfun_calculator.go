// internal/monitor/pumpfun_calculator.go
package monitor

import (
	"context"
	"fmt"

	"github.com/rovshanmuradov/solana-bot/internal/dex"
	"go.uber.org/zap"
)

// pumpFunCalculator реализует расчет PnL, специфичный для Pump.fun DEX
type pumpFunCalculator struct {
	dex    dex.DEX
	logger *zap.Logger
}

// CalculatePnL реализует специфический для bonding curve расчет PnL для Pump.fun
func (c *pumpFunCalculator) CalculatePnL(ctx context.Context, tokenMint string, tokenAmount float64, initialInvestment float64) (*PnLData, error) {
	// Используем type assertion для доступа к специфической реализации
	type bondingCurvePnLCalculator interface {
		CalculateBondingCurvePnL(ctx context.Context, tokenAmount float64, initialInvestment float64) (*dex.BondingCurvePnL, error)
	}

	calculator, ok := c.dex.(bondingCurvePnLCalculator)
	if !ok {
		return nil, fmt.Errorf("Pump.fun DEX does not implement CalculateBondingCurvePnL")
	}

	result, err := calculator.CalculateBondingCurvePnL(ctx, tokenAmount, initialInvestment)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate bonding curve PnL: %w", err)
	}

	// Конвертируем из DEX-специфичного типа в наш универсальный тип
	c.logger.Debug("Pump.fun PnL calculation",
		zap.Float64("token_amount", tokenAmount),
		zap.Float64("price", result.CurrentPrice),
		zap.Float64("theoretical_value", result.TheoreticalValue),
		zap.Float64("sell_estimate", result.SellEstimate),
		zap.Float64("net_pnl", result.NetPnL))

	return &PnLData{
		CurrentPrice:      result.CurrentPrice,
		TheoreticalValue:  result.TheoreticalValue,
		SellEstimate:      result.SellEstimate,
		InitialInvestment: result.InitialInvestment,
		NetPnL:            result.NetPnL,
		PnLPercentage:     result.PnLPercentage,
	}, nil
}
