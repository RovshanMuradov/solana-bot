// internal/monitor/pumpfun_calculator.go
package monitor

import (
	"context"
	"fmt"
	"github.com/rovshanmuradov/solana-bot/internal/dex/pumpfun"

	"github.com/rovshanmuradov/solana-bot/internal/dex"
	"go.uber.org/zap"
)

type pumpFunCalculator struct {
	dex    dex.DEX
	logger *zap.Logger
}

func (c *pumpFunCalculator) CalculatePnL(ctx context.Context, tokenMint string, tokenAmount float64, initialInvestment float64) (*PnLData, error) {
	// Убедимся, что dex реализует интерфейс BondingCurvePnLCalculator
	calculator, ok := c.dex.(pumpfun.BondingCurvePnLCalculator)
	if !ok {
		return nil, fmt.Errorf("DEX does not implement bonding curve calculator")
	}

	result, err := calculator.CalculateBondingCurvePnL(ctx, tokenAmount, initialInvestment)
	if err != nil {
		return nil, fmt.Errorf("error calculating bonding curve PnL: %w", err)
	}

	return &PnLData{
		InitialInvestment: result.InitialInvestment,
		SellEstimate:      result.SellEstimate,
		NetPnL:            result.NetPnL,
		PnLPercentage:     result.PnLPercentage,
	}, nil
}
