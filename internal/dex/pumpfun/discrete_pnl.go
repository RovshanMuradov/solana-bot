// internal/dex/pumpfun/discrete_pnl.go
package pumpfun

import (
	"context"
	"fmt"
	"math"

	"go.uber.org/zap"
)

// PriceTier представляет собой ценовой уровень на bonding curve
type PriceTier struct {
	Price           float64 // Цена в SOL за один токен на этом уровне
	TokensRemaining float64 // Количество токенов, доступных на этом уровне
}

// BondingCurveInfo содержит информацию о текущем состоянии bonding curve
type BondingCurveInfo struct {
	CurrentTierIndex int         // Индекс текущего ценового уровня
	CurrentTierPrice float64     // Текущая цена в SOL
	Tiers            []PriceTier // Ценовые уровни
	FeePercentage    float64     // Комиссия в процентах
}

// DiscreteTokenPnL содержит информацию о PnL с учетом дискретной природы токена
type DiscreteTokenPnL struct {
	CurrentPrice      float64 // Текущая цена токена
	TheoreticalValue  float64 // Теоретическая стоимость (цена * количество)
	SellEstimate      float64 // Оценка реальной выручки при продаже
	InitialInvestment float64 // Начальная инвестиция
	NetPnL            float64 // Чистый PnL (SellEstimate - InitialInvestment)
	PnLPercentage     float64 // Процент PnL
}

// calculateSellValue рассчитывает фактическую выручку от продажи токенов с учетом
// ступенчатого падения цены по дискретным уровням bonding curve
func calculateSellValue(tokenAmount float64, curveInfo *BondingCurveInfo) float64 {
	remainingTokens := tokenAmount
	totalSol := 0.0

	// Начинаем с текущего уровня и двигаемся вниз
	for i := curveInfo.CurrentTierIndex; i >= 0 && remainingTokens > 0; i-- {
		tier := curveInfo.Tiers[i]

		// Сколько токенов продаем на этом уровне
		tokensToSellAtTier := math.Min(remainingTokens, tier.TokensRemaining)

		// SOL, полученный от продажи на этом уровне
		solFromTier := tokensToSellAtTier * tier.Price

		// Вычитаем комиссию
		netSolFromTier := solFromTier * (1.0 - curveInfo.FeePercentage)

		// Добавляем к общей сумме
		totalSol += netSolFromTier

		// Уменьшаем количество оставшихся токенов
		remainingTokens -= tokensToSellAtTier
	}

	// Округляем до 6 знаков после запятой (микро SOL)
	totalSol = math.Floor(totalSol*1e6) / 1e6

	return totalSol
}

// CalculateDiscretePnL вычисляет PnL для дискретной системы Pump.fun
func (d *DEX) CalculateDiscretePnL(ctx context.Context, tokenAmount float64, initialInvestment float64) (*DiscreteTokenPnL, error) {
	// Получить данные о текущем состоянии bonding curve
	bondingCurve, _, err := d.deriveBondingCurveAccounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to derive bonding curve addresses: %w", err)
	}

	// Получить данные аккаунта
	bondingCurveData, err := d.FetchBondingCurveAccount(ctx, bondingCurve)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch bonding curve data: %w", err)
	}

	// Текущая цена токена
	currentPrice := float64(bondingCurveData.VirtualSolReserves) / float64(bondingCurveData.VirtualTokenReserves)
	currentPrice = math.Floor(currentPrice*1e9) / 1e9 // Округляем до 9 знаков после запятой

	// Создаем инфо о bonding curve
	// Определяем текущий уровень и модель ценовых уровней
	const priceIncrement = 0.001 // Инкремент цены между уровнями, обычно 0.001 SOL
	currentTierIndex := int(math.Floor(currentPrice / priceIncrement))

	// Создаем массив ценовых уровней от текущего вниз
	tiers := make([]PriceTier, currentTierIndex+1)

	// Заполняем текущий уровень реальной ценой
	tiers[currentTierIndex] = PriceTier{
		Price:           currentPrice,
		TokensRemaining: 20.0, // Приблизительное количество токенов на уровень
	}

	// Заполняем все предыдущие уровни с шагом priceIncrement
	for i := currentTierIndex - 1; i >= 0; i-- {
		tierPrice := priceIncrement * float64(i+1)
		tiers[i] = PriceTier{
			Price:           tierPrice,
			TokensRemaining: 20.0, // Стандартное количество токенов на уровень в Pump.fun
		}
	}

	// Получаем комиссию (по умолчанию 1%)
	const defaultFeePercentage = 0.01

	// Создаем info о bonding curve
	curveInfo := &BondingCurveInfo{
		CurrentTierIndex: currentTierIndex,
		CurrentTierPrice: currentPrice,
		Tiers:            tiers,
		FeePercentage:    defaultFeePercentage,
	}

	// Расчет теоретической стоимости по текущей цене
	theoreticalValue := tokenAmount * currentPrice

	// Расчет ожидаемой выручки от продажи с учетом ступенчатого спуска
	sellEstimate := calculateSellValue(tokenAmount, curveInfo)

	// Расчет чистого PnL
	netPnL := sellEstimate - initialInvestment

	// Расчет процента PnL
	pnlPercentage := 0.0
	if initialInvestment > 0 {
		pnlPercentage = (netPnL / initialInvestment) * 100
	}

	d.logger.Debug("Calculated discrete PnL",
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