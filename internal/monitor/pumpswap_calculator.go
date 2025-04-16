// internal/monitor/pumpswap_calculator.go
package monitor

import (
	"context"
	"fmt"
	"math"

	"github.com/rovshanmuradov/solana-bot/internal/dex"
	"go.uber.org/zap"
)

// pumpSwapCalculator реализует расчет PnL, специфичный для Pump.swap DEX
type pumpSwapCalculator struct {
	dex    dex.DEX
	logger *zap.Logger
}

// CalculatePnL реализует специфический для AMM пула расчет PnL для Pump.swap
func (c *pumpSwapCalculator) CalculatePnL(ctx context.Context, tokenMint string, tokenAmount float64, initialInvestment float64) (*PnLData, error) {
	// Получаем текущую цену
	price, err := c.dex.GetTokenPrice(ctx, tokenMint)
	if err != nil {
		return nil, fmt.Errorf("failed to get token price: %w", err)
	}

	theoreticalValue := tokenAmount * price

	// Для Pump.swap учитываем комиссию пула при расчете выручки от продажи
	// Обычно комиссия пула составляет 0.3% для стандартных AMM DEX
	const poolFeePercentage = 0.003 // 0.3%

	// Применяем комиссию к теоретической стоимости
	sellEstimate := theoreticalValue * (1.0 - poolFeePercentage)

	// Рассчитываем чистый PnL
	netPnL := sellEstimate - initialInvestment

	// Рассчитываем процент PnL
	pnlPercentage := 0.0
	if initialInvestment > 0 {
		pnlPercentage = (netPnL / initialInvestment) * 100
	} else if netPnL > 0 {
		pnlPercentage = math.Inf(1)
	}

	c.logger.Debug("Pump.swap PnL calculation",
		zap.Float64("token_amount", tokenAmount),
		zap.Float64("price", price),
		zap.Float64("pool_fee_percentage", poolFeePercentage),
		zap.Float64("theoretical_value", theoreticalValue),
		zap.Float64("sell_estimate", sellEstimate),
		zap.Float64("net_pnl", netPnL))

	return &PnLData{
		CurrentPrice:      price,
		TheoreticalValue:  theoreticalValue,
		SellEstimate:      sellEstimate,
		InitialInvestment: initialInvestment,
		NetPnL:            netPnL,
		PnLPercentage:     pnlPercentage,
	}, nil
}
