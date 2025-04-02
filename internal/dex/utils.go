// =============================
// File: internal/dex/utils.go
// =============================
package dex

import (
	"context"
	"github.com/gagliardetto/solana-go"
	"math"
)

// convertToTokenUnits конвертирует человекочитаемое представление в базовые единицы токена
func convertToTokenUnits(ctx context.Context, dex interface{}, tokenMint string, amount float64, defaultDecimals uint8) (uint64, error) {
	decimals := float64(defaultDecimals)

	// Пытаемся определить точность токена, если доступно
	if precisionProvider, ok := dex.(interface {
		DetermineTokenPrecision(ctx context.Context, mint solana.PublicKey) (uint8, error)
	}); ok {
		mint, err := solana.PublicKeyFromBase58(tokenMint)
		if err == nil {
			precision, err := precisionProvider.DetermineTokenPrecision(ctx, mint)
			if err == nil {
				decimals = float64(precision)
			}
		}
	}

	// Конвертируем в базовые единицы токена
	return uint64(amount * math.Pow(10, decimals)), nil
}