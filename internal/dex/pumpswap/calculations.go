// =============================
// File: internal/dex/pumpswap/calculations.go
// =============================
package pumpswap

import (
	"context"
	"fmt"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/token"
	"go.uber.org/zap"
	"math/big"
)

// calculateSwapAmounts вычисляет параметры для операции свапа в зависимости от типа операции (покупка/продажа).
func (d *DEX) calculateSwapAmounts(pool *PoolInfo, isBuy bool, amount uint64, slippage float64) *SwapAmounts {
	outputAmount, price := d.poolManager.CalculateSwapQuote(pool, amount, false)

	if isBuy {
		return calculateBuySwap(amount, outputAmount, slippage, price, d.logger)
	}

	return calculateSellSwap(amount, outputAmount, slippage, price, d.logger)
}

// calculateBuySwap вычисляет параметры для операции покупки токена.
func calculateBuySwap(input, output uint64, slippage, price float64, logger *zap.Logger) *SwapAmounts {
	maxAmountWithBuffer := uint64(float64(input) * (1.0 + slippage/100.0))
	minOut := uint64(float64(output) * (1.0 - slippage/100.0))

	logger.Debug("Buy swap calculation",
		zap.Uint64("input_amount", input),
		zap.Uint64("max_amount_with_buffer", maxAmountWithBuffer),
		zap.Uint64("expected_output", output),
		zap.Uint64("min_out_amount", minOut),
		zap.Float64("price", price))

	return &SwapAmounts{BaseAmount: output, QuoteAmount: maxAmountWithBuffer, Price: price}
}

// calculateSellSwap вычисляет параметры для операции продажи токена.
func calculateSellSwap(input, output uint64, slippage, price float64, logger *zap.Logger) *SwapAmounts {
	minOut := uint64(float64(output) * (1.0 - slippage/100.0))

	logger.Debug("Sell swap calculation",
		zap.Uint64("input_amount", input),
		zap.Uint64("expected_output", output),
		zap.Uint64("min_out_amount", minOut),
		zap.Float64("price", price))

	return &SwapAmounts{BaseAmount: input, QuoteAmount: minOut, Price: price}
}

// calculateOutput вычисляет выходное количество токенов для операции свапа по формуле пула ликвидности.
//
// Функция реализует математическую формулу Constant Product AMM (Automated Market Maker):
// outputAmount = y * a * feeFactor / (x + a * feeFactor), где:
// - x - резервы входного токена
// - y - резервы выходного токена
// - a - входное количество
// - feeFactor - коэффициент комиссии (1 - fee)
func calculateOutput(reserves, otherReserves, amount uint64, feeFactor float64) uint64 {
	x := new(big.Float).SetUint64(reserves)
	y := new(big.Float).SetUint64(otherReserves)
	a := new(big.Float).SetUint64(amount)

	// Apply fee to input amount
	a.Mul(a, big.NewFloat(feeFactor))

	// Formula: outputAmount = y * a / (x + a)
	numerator := new(big.Float).Mul(y, a)
	denominator := new(big.Float).Add(x, a)
	result := new(big.Float).Quo(numerator, denominator)

	output, _ := result.Uint64()
	return output
}

// getTokenDecimals получает количество десятичных знаков для токена.
func (d *DEX) getTokenDecimals(ctx context.Context, mint solana.PublicKey, defaultDec uint8) uint8 {
	dec, err := d.DetermineTokenPrecision(ctx, mint)
	if err != nil {
		d.logger.Warn("Using default decimals", zap.Error(err), zap.String("mint", mint.String()))
		return defaultDec
	}
	return dec
}

// DetermineTokenPrecision получает количество десятичных знаков для данного токена.
func (d *DEX) DetermineTokenPrecision(ctx context.Context, mint solana.PublicKey) (uint8, error) {
	var mintInfo token.Mint
	err := d.client.GetAccountDataInto(ctx, mint, &mintInfo)
	if err != nil {
		return 0, fmt.Errorf("failed to get mint info: %w", err)
	}

	return mintInfo.Decimals, nil
}
