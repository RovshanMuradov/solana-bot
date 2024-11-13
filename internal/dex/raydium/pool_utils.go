// internal/dex/raydium/pool_utils.go
package raydium

import (
	"fmt"
	"math"

	"github.com/gagliardetto/solana-go"
	"go.uber.org/zap"
)

// ValidatePoolAccounts проверяет валидность аккаунтов пула
func ValidatePoolAccounts(pool *Pool) error {
	// Проверяем существование всех необходимых аккаунтов
	accounts := []solana.PublicKey{
		pool.ID,
		pool.Authority,
		pool.BaseMint,
		pool.QuoteMint,
		pool.BaseVault,
		pool.QuoteVault,
	}

	for _, acc := range accounts {
		if acc.IsZero() {
			return fmt.Errorf("invalid pool account: %s is zero", acc.String())
		}
	}

	return nil
}

// CalculateSwapAmounts вычисляет количество выходных токенов с учетом слиппажа
func CalculateSwapAmounts(pool *Pool, amountIn uint64, slippageBps uint16) *SwapAmounts {
	// Расчет выходного количества токенов
	amountOut := (amountIn * pool.State.QuoteReserve) / (pool.State.BaseReserve + amountIn)

	// Расчет минимального выходного количества с учетом слиппажа
	slippage := (amountOut * uint64(slippageBps)) / 10000
	minAmountOut := amountOut - slippage

	return &SwapAmounts{
		AmountIn:     amountIn,
		AmountOut:    amountOut,
		MinAmountOut: minAmountOut,
	}
}

// IsPoolActive проверяет, активен ли пул
func IsPoolActive(pool *Pool) bool {
	return pool.State.Status == 1 && // активный статус
		pool.State.BaseReserve > 0 &&
		pool.State.QuoteReserve > 0
}

// GetPriceImpact вычисляет влияние сделки на цену в процентах
func GetPriceImpact(pool *Pool, amountIn uint64) float64 {
	if pool.State.BaseReserve == 0 || pool.State.QuoteReserve == 0 {
		return 0
	}

	currentPrice := float64(pool.State.QuoteReserve) / float64(pool.State.BaseReserve)
	newBaseReserve := float64(pool.State.BaseReserve + amountIn)
	newPrice := float64(pool.State.QuoteReserve) / newBaseReserve

	return (currentPrice - newPrice) / currentPrice * 100
}

// CalculateSwapImpact рассчитывает влияние свапа на цену
func CalculateSwapImpact(pool *Pool, amountIn uint64) (float64, error) {
	if pool == nil {
		return 0, fmt.Errorf("pool cannot be nil")
	}

	// 1. Получаем текущие резервы пула
	currentBaseReserve := float64(pool.State.BaseReserve)
	currentQuoteReserve := float64(pool.State.QuoteReserve)

	if currentBaseReserve == 0 || currentQuoteReserve == 0 {
		return 0, fmt.Errorf("invalid pool reserves: base=%v, quote=%v",
			currentBaseReserve, currentQuoteReserve)
	}

	// 2. Рассчитываем изменение цены после свапа
	// Текущая цена = quote_reserve / base_reserve
	currentPrice := currentQuoteReserve / currentBaseReserve

	// Новые резервы после свапа
	newBaseReserve := currentBaseReserve + float64(amountIn)
	// k = x * y - константа пула
	k := currentBaseReserve * currentQuoteReserve
	newQuoteReserve := k / newBaseReserve

	// Новая цена = new_quote_reserve / new_base_reserve
	newPrice := newQuoteReserve / newBaseReserve

	// Расчет процента изменения цены
	priceImpact := ((currentPrice - newPrice) / currentPrice) * 100

	// 3. Проверяем, не превышает ли impact допустимый предел
	const maxImpact = 10.0 // максимально допустимое влияние на цену в процентах
	if math.Abs(priceImpact) > maxImpact {
		return priceImpact, fmt.Errorf("price impact too high: %.2f%% (max allowed: %.2f%%)",
			math.Abs(priceImpact), maxImpact)
	}

	// 4. Логируем результаты расчета
	log := zap.L().With(
		zap.String("pool_id", pool.ID.String()),
		zap.Float64("current_price", currentPrice),
		zap.Float64("new_price", newPrice),
		zap.Float64("impact_percentage", priceImpact),
		zap.Uint64("amount_in", amountIn),
	)

	if math.Abs(priceImpact) > 5.0 {
		log.Warn("high price impact detected")
	} else {
		log.Debug("price impact calculated")
	}

	return priceImpact, nil
}

// CheckLiquiditySufficiency проверяет достаточность ликвидности
func CheckLiquiditySufficiency(pool *Pool, amountIn uint64) error {
	if pool == nil {
		return fmt.Errorf("pool cannot be nil")
	}

	// 1. Проверяем общую ликвидность пула
	totalLiquidity := pool.State.BaseReserve + pool.State.QuoteReserve
	const minTotalLiquidity = uint64(1000000) // минимальная общая ликвидность в lamports

	if totalLiquidity < minTotalLiquidity {
		return fmt.Errorf("insufficient total liquidity: %d (min required: %d)",
			totalLiquidity, minTotalLiquidity)
	}

	// 2. Сравниваем размер свапа с резервами
	// Свап не должен превышать определенный процент от резервов
	const maxSwapToReserveRatio = 0.1 // максимальное соотношение размера свапа к резервам (10%)

	baseRatio := float64(amountIn) / float64(pool.State.BaseReserve)
	if baseRatio > maxSwapToReserveRatio {
		return fmt.Errorf("swap amount too large compared to base reserve: %.2f%% (max: %.2f%%)",
			baseRatio*100, maxSwapToReserveRatio*100)
	}

	// 3. Проверяем глубину пула
	// Рассчитываем соотношение резервов для оценки баланса пула
	reserveRatio := float64(pool.State.BaseReserve) / float64(pool.State.QuoteReserve)
	const maxReserveImbalance = 5.0 // максимальное допустимое отклонение от 1:1

	if reserveRatio > maxReserveImbalance || reserveRatio < 1/maxReserveImbalance {
		return fmt.Errorf("pool reserves too imbalanced: ratio=%.2f (max allowed: %.2f)",
			reserveRatio, maxReserveImbalance)
	}

	// 4. Проверяем возможные проскальзывания
	// Рассчитываем ожидаемое проскальзывание на основе размера свапа
	// Используем формулу: slip = (amount_in * amount_in) / (reserve * 4)
	slippage := (float64(amountIn) * float64(amountIn)) /
		(float64(pool.State.BaseReserve) * 4)

	const maxSlippage = 0.05 // максимальное допустимое проскальзывание (5%)
	if slippage > maxSlippage {
		return fmt.Errorf("expected slippage too high: %.2f%% (max: %.2f%%)",
			slippage*100, maxSlippage*100)
	}

	// Дополнительные проверки
	// Проверяем минимальный размер резервов для каждого токена
	const minReserveSize = uint64(100000) // минимальный размер резерва в lamports
	if pool.State.BaseReserve < minReserveSize || pool.State.QuoteReserve < minReserveSize {
		return fmt.Errorf("individual reserve too small: base=%d, quote=%d (min: %d)",
			pool.State.BaseReserve, pool.State.QuoteReserve, minReserveSize)
	}

	// Логируем результаты проверки
	zap.L().Debug("liquidity check passed",
		zap.String("pool_id", pool.ID.String()),
		zap.Uint64("total_liquidity", totalLiquidity),
		zap.Float64("reserve_ratio", reserveRatio),
		zap.Float64("expected_slippage", slippage),
		zap.Uint64("amount_in", amountIn))

	return nil
}
