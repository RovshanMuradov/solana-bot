// internal/dex/pumpfun/interfaces.go
package pumpfun

import "context"

type BondingCurvePnLCalculator interface {
	CalculateBondingCurvePnL(ctx context.Context, tokenAmount float64, initialInvestment float64) (*BondingCurvePnL, error)
}
type TokenValuation interface {
	CalculateTokenPrice(ctx context.Context, data *BondingCurve) (float64, error)
	CalculateSellValue(ctx context.Context, tokenAmount float64, data *BondingCurve) (float64, error)
}
