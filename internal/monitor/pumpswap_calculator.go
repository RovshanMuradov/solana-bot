// internal/monitor/pumpswap_calculator.go
package monitor

import (
	"context"
	"fmt"
	"github.com/rovshanmuradov/solana-bot/internal/dex"
	"github.com/rovshanmuradov/solana-bot/internal/dex/pumpswap"
	"go.uber.org/zap"
)

// pumpSwapCalculator реализует расчет PnL, специфичный для Pump.swap DEX
type pumpSwapCalculator struct {
	dex    dex.DEX
	logger *zap.Logger
}

// CalculatePnL делегирует расчёт PnL к внутреннему TokenCalculator через интерфейс PnLCalculatorInterface.
func (c *pumpSwapCalculator) CalculatePnL(
	ctx context.Context,
	tokenMint string,
	tokenAmount float64,
	initialInvestment float64,
) (*PnLData, error) {
	// Приводим адаптер DEX к PnLCalculatorInterface
	pnlCalc, ok := c.dex.(pumpswap.PnLCalculatorInterface)
	if !ok {
		return nil, fmt.Errorf("DEX adapter does not implement PnLCalculatorInterface for Pump.Swap")
	}

	// Вызов внутреннего CalculatePnL
	res, err := pnlCalc.CalculatePnL(ctx, tokenAmount, initialInvestment)
	if err != nil {
		c.logger.Error("TokenCalculator.CalculatePnL error", zap.Error(err))
		return nil, err
	}

	// Преобразуем результат TokenPnL в PnLData
	return &PnLData{
		InitialInvestment: res.InitialInvestment,
		SellEstimate:      res.SellEstimate,
		NetPnL:            res.NetPnL,
		PnLPercentage:     res.PnLPercentage,
	}, nil
}
