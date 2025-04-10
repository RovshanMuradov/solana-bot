// internal/dex/pumpfun/discrete_pnl.go
package pumpfun

import (
	"context"
	"fmt"
	"math"

	"go.uber.org/zap"
)

// DiscreteTokenPnL содержит информацию о прибыли/убытке (PnL) токена
type DiscreteTokenPnL struct {
	CurrentPrice      float64 // Текущая цена токена (SOL за токен)
	TheoreticalValue  float64 // Теоретическая стоимость текущей позиции: токены * CurrentPrice
	SellEstimate      float64 // Приблизительная выручка при продаже
	InitialInvestment float64 // Первоначальные вложения в SOL
	NetPnL            float64 // Чистая прибыль/убыток: SellEstimate - InitialInvestment
	PnLPercentage     float64 // Процент PnL от начальных вложений
}

// BondingCurveParams содержит параметры для расчёта bonding curve Pump.fun
type BondingCurveParams struct {
	InitialSolReserve float64 // x₀, начальное значение виртуального резерва SOL (например, 30 SOL)
	ConstantK         float64 // K, константа в модели (например, 32190005730)
	FeePercentage     float64 // комиссия (например, 1% -> 0.01)
}

// GetDefaultBondingCurveParams возвращает параметры по умолчанию
func GetDefaultBondingCurveParams() *BondingCurveParams {
	return &BondingCurveParams{
		InitialSolReserve: 30.0,
		ConstantK:         32190005730.0,
		FeePercentage:     0.01,
	}
}

// CalculateTokenPrice рассчитывает цену токена по модели bonding curve.
// Формула: price = (current_virtual_SOL)² / K, где current_virtual_SOL – текущее значение виртуального резерва SOL (в SOL).
func (d *DEX) CalculateTokenPrice(ctx context.Context, bondingCurveData *BondingCurve) (float64, error) {
	params := GetDefaultBondingCurveParams()

	// Перевод виртуальных SOL из лампортов в SOL:
	currentVirtualSOL := float64(bondingCurveData.VirtualSolReserves) / 1e9

	// Если текущее значение меньше начального, используем InitialSolReserve:
	if currentVirtualSOL < params.InitialSolReserve {
		d.logger.Warn("Virtual SOL reserve below initial, using initial reserve",
			zap.Float64("current_virtual_sol", currentVirtualSOL),
			zap.Float64("initial_sol_reserve", params.InitialSolReserve))
		currentVirtualSOL = params.InitialSolReserve
	}

	// Расчёт цены по формуле: price = (currentVirtualSOL)² / ConstantK
	price := math.Pow(currentVirtualSOL, 2) / params.ConstantK

	// Применяем нижнюю границу: не менее 1 nano-SOL (1e-9 SOL)
	if price < 1e-9 {
		d.logger.Debug("Price too low, setting to minimum of 1 nano-SOL", zap.Float64("raw_price", price))
		price = 1e-9
	}

	// Если цена получилась слишком высокой (например, выше логически допустимой границы для bonding curve),
	// можно задать верхний предел. Здесь, если значение > 0.001 SOL, мы ограничиваем его.
	if price > 0.001 {
		d.logger.Warn("Price too high, capping at 0.001 SOL", zap.Float64("raw_price", price))
		price = 0.001
	}

	d.logger.Debug("Calculated token price using bonding curve",
		zap.Float64("current_virtual_sol", currentVirtualSOL),
		zap.Float64("calculated_price", price))

	return price, nil
}

// CalculateSellValue вычисляет оценку SOL (выручку) от продажи заданного количества токенов,
// исходя из текущей цены и с учётом комиссии.
func (d *DEX) CalculateSellValue(ctx context.Context, tokenAmount float64, bondingCurveData *BondingCurve) (float64, error) {
	// Получаем текущую цену по модели bonding curve
	currentPrice, err := d.CalculateTokenPrice(ctx, bondingCurveData)
	if err != nil {
		return 0, fmt.Errorf("failed to calculate current price: %w", err)
	}

	// Базовая стоимость продажи (без учета комиссии)
	baseValue := tokenAmount * currentPrice

	// Применяем комиссию (например, 1%)
	feePercentage := 0.01
	sellEstimate := baseValue * (1.0 - feePercentage)

	// Ограничение: продажа не должна превышать 95% виртуального SOL-резерва,
	// чтобы не «разоружить» пул ликвидности.
	currentVirtualSOL := float64(bondingCurveData.VirtualSolReserves) / 1e9
	maxPossibleSell := currentVirtualSOL * 0.95
	if sellEstimate > maxPossibleSell {
		sellEstimate = maxPossibleSell
		d.logger.Debug("Sell estimate capped to 95% of virtual SOL reserve", zap.Float64("sellEstimate", sellEstimate))
	}

	d.logger.Debug("Sell estimate calculation",
		zap.Float64("tokenAmount", tokenAmount),
		zap.Float64("currentPrice", currentPrice),
		zap.Float64("sellEstimate", sellEstimate),
		zap.Float64("virtual_sol", currentVirtualSOL))
	return sellEstimate, nil
}

// CalculateDiscretePnL вычисляет PnL (прибыль/убыток) по количеству токенов и первоначальным инвестициям.
func (d *DEX) CalculateDiscretePnL(ctx context.Context, tokenAmount float64, initialInvestment float64) (*DiscreteTokenPnL, error) {
	// Получаем данные bonding curve через функции deriveBondingCurveAccounts и FetchBondingCurveAccount
	bondingCurve, _, err := d.deriveBondingCurveAccounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to derive bonding curve accounts: %w", err)
	}

	bondingCurveData, err := d.FetchBondingCurveAccount(ctx, bondingCurve)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch bonding curve data: %w", err)
	}

	// Расчёт текущей цены
	currentPrice, err := d.CalculateTokenPrice(ctx, bondingCurveData)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate current price: %w", err)
	}

	// Теоретическая стоимость (tokens * currentPrice)
	theoreticalValue := tokenAmount * currentPrice

	// Оценка продажи с учетом комиссии и ограничений
	sellEstimate, err := d.CalculateSellValue(ctx, tokenAmount, bondingCurveData)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate sell estimate: %w", err)
	}

	netPnL := sellEstimate - initialInvestment
	pnlPercentage := 0.0
	if initialInvestment > 0 {
		pnlPercentage = (netPnL / initialInvestment) * 100
	}

	d.logger.Debug("Discrete PnL calculation",
		zap.Float64("current_price", currentPrice),
		zap.Float64("theoretical_value", theoreticalValue),
		zap.Float64("sell_estimate", sellEstimate),
		zap.Float64("net_pnl", netPnL),
		zap.Float64("pnl_percentage", pnlPercentage))

	return &DiscreteTokenPnL{
		CurrentPrice:      currentPrice,
		TheoreticalValue:  theoreticalValue,
		SellEstimate:      sellEstimate,
		InitialInvestment: initialInvestment,
		NetPnL:            netPnL,
		PnLPercentage:     pnlPercentage,
	}, nil
}
