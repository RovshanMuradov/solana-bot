package raydium

import (
	"fmt"

	"github.com/gagliardetto/solana-go"
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
