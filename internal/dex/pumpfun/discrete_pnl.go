// internal/dex/pumpfun/discrete_pnl.go
// Package pumpfun реализует взаимодействие с Pump.fun DEX на базе Solana.
package pumpfun

import (
	"context"
	"fmt"
	"math"

	"go.uber.org/zap"
)

// PriceTier представляет собой ценовой уровень на bonding curve Pump.fun.
type PriceTier struct {
	Price           float64
	TokensRemaining float64
}

// BondingCurveInfo содержит структурированную информацию о текущем состоянии
// bonding curve, включая иерархию ценовых уровней и их параметры.
type BondingCurveInfo struct {
	CurrentTierIndex int
	CurrentTierPrice float64
	Tiers            []PriceTier
	FeePercentage    float64
}

// DiscreteTokenPnL содержит информацию о прибыли и убытках (PnL) токена
type DiscreteTokenPnL struct {
	CurrentPrice      float64
	TheoreticalValue  float64
	SellEstimate      float64
	InitialInvestment float64
	NetPnL            float64
	PnLPercentage     float64
}

// calculateSellValue рассчитывает фактическую выручку от продажи токенов с учетом
// ступенчатого падения цены по дискретным уровням bonding curve.
func calculateSellValue(tokenAmount float64, curveInfo *BondingCurveInfo) float64 {
	// Инициализируем оставшееся количество токенов для продажи
	remainingTokens := tokenAmount

	// Инициализируем общую сумму SOL, которую получим от продажи
	totalSol := 0.0

	// Начинаем с текущего уровня и двигаемся вниз по уровням
	// Продажа начинается с высшего уровня цены и продолжается на более низких уровнях
	for i := curveInfo.CurrentTierIndex; i >= 0 && remainingTokens > 0; i-- {
		tier := curveInfo.Tiers[i]

		// Определяем сколько токенов будет продано на текущем уровне цены
		// Берем минимум между оставшимися токенами и емкостью текущего уровня
		tokensToSellAtTier := math.Min(remainingTokens, tier.TokensRemaining)

		// Рассчитываем сумму SOL, которую получим от продажи на этом уровне
		// до вычета комиссий
		solFromTier := tokensToSellAtTier * tier.Price

		// Вычитаем комиссию протокола
		netSolFromTier := solFromTier * (1.0 - curveInfo.FeePercentage)

		// Добавляем чистую выручку с этого уровня к общей сумме
		totalSol += netSolFromTier

		// Уменьшаем количество оставшихся для продажи токенов
		remainingTokens -= tokensToSellAtTier
	}

	// Округляем результат до 6 знаков после запятой (микро SOL)
	// для обеспечения точности финансовых расчетов
	totalSol = math.Floor(totalSol*1e6) / 1e6

	return totalSol
}

// CalculateDiscretePnL вычисляет детальный анализ прибыли и убытков для токенов
// в экосистеме Pump.fun с учетом ступенчатой структуры bonding curve.
func (d *DEX) CalculateDiscretePnL(ctx context.Context, tokenAmount float64, initialInvestment float64) (*DiscreteTokenPnL, error) {
	// Шаг 1: Получаем адреса аккаунтов bonding curve для токена
	bondingCurve, _, err := d.deriveBondingCurveAccounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to derive bonding curve addresses: %w", err)
	}

	// Шаг 2: Загружаем данные аккаунта из блокчейна
	bondingCurveData, err := d.FetchBondingCurveAccount(ctx, bondingCurve)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch bonding curve data: %w", err)
	}

	// Шаг 3: Вычисляем текущую цену токена на основе соотношения резервов
	// В Pump.fun цена определяется отношением виртуальных SOL резервов к токеновым
	currentPrice := float64(bondingCurveData.VirtualSolReserves) / float64(bondingCurveData.VirtualTokenReserves)

	// Округляем цену до 9 знаков после запятой (нано SOL)
	currentPrice = math.Floor(currentPrice*1e9) / 1e9

	// Шаг 4: Определяем шаг цены и текущий индекс уровня
	// В Pump.fun стандартный шаг цены между уровнями составляет 0.001 SOL
	const priceIncrement = 0.001
	currentTierIndex := int(math.Floor(currentPrice / priceIncrement))

	// Шаг 5: Создаем массив ценовых уровней от нулевого до текущего
	tiers := make([]PriceTier, currentTierIndex+1)

	// Шаг 6: Заполняем текущий уровень реальной ценой и стандартным объемом
	tiers[currentTierIndex] = PriceTier{
		Price:           currentPrice,
		TokensRemaining: 20.0, // Приблизительное количество токенов на уровень
	}

	// Шаг 7: Заполняем все предыдущие уровни с шагом priceIncrement
	// Каждый предыдущий уровень имеет более низкую цену
	for i := currentTierIndex - 1; i >= 0; i-- {
		tierPrice := priceIncrement * float64(i+1)
		tiers[i] = PriceTier{
			Price:           tierPrice,
			TokensRemaining: 20.0, // Стандартное количество токенов на уровень в Pump.fun
		}
	}

	// Шаг 8: Устанавливаем стандартную комиссию протокола
	// TODO: В будущем получать точное значение из GlobalAccount.FeeBasisPoints / 10000.0
	const defaultFeePercentage = 0.01 // 1%

	// Шаг 9: Собираем информацию о bonding curve в единую структуру
	curveInfo := &BondingCurveInfo{
		CurrentTierIndex: currentTierIndex,
		CurrentTierPrice: currentPrice,
		Tiers:            tiers,
		FeePercentage:    defaultFeePercentage,
	}

	// Шаг 10: Рассчитываем теоретическую стоимость по текущей цене
	// (без учета ступенчатого спуска при продаже)
	theoreticalValue := tokenAmount * currentPrice

	// Шаг 11: Рассчитываем реалистичную оценку выручки от продажи
	// с учетом ступенчатого спуска по уровням цены
	sellEstimate := calculateSellValue(tokenAmount, curveInfo)

	// Шаг 12: Рассчитываем чистый PnL как разницу между оценкой
	// выручки и начальной инвестицией
	netPnL := sellEstimate - initialInvestment

	// Шаг 13: Рассчитываем процент PnL относительно начальной инвестиции
	pnlPercentage := 0.0
	if initialInvestment > 0 {
		pnlPercentage = (netPnL / initialInvestment) * 100
	}

	// Шаг 14: Логируем результаты для отладки
	d.logger.Debug("Calculated discrete PnL",
		zap.Float64("current_price", currentPrice),
		zap.Float64("theoretical_value", theoreticalValue),
		zap.Float64("sell_estimate", sellEstimate),
		zap.Float64("net_pnl", netPnL),
		zap.Float64("pnl_percentage", pnlPercentage))

	// Шаг 15: Создаем и возвращаем структуру с результатами PnL
	return &DiscreteTokenPnL{
		CurrentPrice:      currentPrice,
		TheoreticalValue:  theoreticalValue,
		SellEstimate:      sellEstimate,
		InitialInvestment: initialInvestment,
		NetPnL:            netPnL,
		PnLPercentage:     pnlPercentage,
	}, nil
}
