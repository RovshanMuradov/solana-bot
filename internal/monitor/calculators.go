// Файл содержит интерфейсы и реализации для вычислений, специфичных для разных DEX
package monitor

import (
	"context"
	"fmt"
	"math"

	"github.com/rovshanmuradov/solana-bot/internal/dex"
	"go.uber.org/zap"
)

// PnLData содержит универсальную информацию о прибыли/убытке (PnL) токена
type PnLData struct {
	CurrentPrice      float64 // Текущая цена токена (SOL за токен)
	TheoreticalValue  float64 // Теоретическая стоимость текущей позиции: токены * CurrentPrice
	SellEstimate      float64 // Приблизительная выручка при продаже (с учетом комиссии)
	InitialInvestment float64 // Первоначальные вложения в SOL
	NetPnL            float64 // Чистая прибыль/убыток: SellEstimate - InitialInvestment
	PnLPercentage     float64 // Процент PnL от начальных вложений
}

// PnLCalculator определяет интерфейс для расчета показателей прибыли и убытка для токенов
type PnLCalculator interface {
	// CalculatePnL вычисляет показатели прибыли и убытка для токенов
	CalculatePnL(ctx context.Context, tokenMint string, tokenAmount float64, initialInvestment float64) (*PnLData, error)
}

// calculatorRegistry сопоставляет типы DEX с соответствующими калькуляторами
var calculatorRegistry = make(map[string]func(dex.DEX, *zap.Logger) PnLCalculator)

// RegisterCalculator регистрирует фабрику калькуляторов для определенного типа DEX
func RegisterCalculator(dexName string, factory func(dex.DEX, *zap.Logger) PnLCalculator) {
	calculatorRegistry[dexName] = factory
}

// GetCalculator возвращает подходящий калькулятор для данного DEX
// Возвращает ошибку, если калькулятор не найден
func GetCalculator(d dex.DEX, logger *zap.Logger) (PnLCalculator, error) {
	factory, exists := calculatorRegistry[d.GetName()]
	if !exists {
		return nil, fmt.Errorf("no calculator registered for DEX: %s", d.GetName())
	}
	return factory(d, logger), nil
}

// pumpFunCalculator реализует расчет PnL, специфичный для Pump.fun DEX
type pumpFunCalculator struct {
	dex    dex.DEX
	logger *zap.Logger
}

// CalculatePnL реализует специфический для bonding curve расчет PnL для Pump.fun
func (c *pumpFunCalculator) CalculatePnL(ctx context.Context, tokenMint string, tokenAmount float64, initialInvestment float64) (*PnLData, error) {
	// Используем type assertion для доступа к специфической реализации
	type bondingCurvePnLCalculator interface {
		CalculateBondingCurvePnL(ctx context.Context, tokenAmount float64, initialInvestment float64) (*dex.BondingCurvePnL, error)
	}

	calculator, ok := c.dex.(bondingCurvePnLCalculator)
	if !ok {
		return nil, fmt.Errorf("Pump.fun DEX does not implement CalculateBondingCurvePnL")
	}

	result, err := calculator.CalculateBondingCurvePnL(ctx, tokenAmount, initialInvestment)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate bonding curve PnL: %w", err)
	}

	// Конвертируем из DEX-специфичного типа в наш универсальный тип
	c.logger.Debug("Pump.fun PnL calculation",
		zap.Float64("token_amount", tokenAmount),
		zap.Float64("price", result.CurrentPrice),
		zap.Float64("theoretical_value", result.TheoreticalValue),
		zap.Float64("sell_estimate", result.SellEstimate),
		zap.Float64("net_pnl", result.NetPnL))

	return &PnLData{
		CurrentPrice:      result.CurrentPrice,
		TheoreticalValue:  result.TheoreticalValue,
		SellEstimate:      result.SellEstimate,
		InitialInvestment: result.InitialInvestment,
		NetPnL:            result.NetPnL,
		PnLPercentage:     result.PnLPercentage,
	}, nil
}

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

// init регистрирует калькуляторы при загрузке пакета
func init() {
	RegisterCalculator("Pump.fun", func(d dex.DEX, logger *zap.Logger) PnLCalculator {
		return &pumpFunCalculator{dex: d, logger: logger}
	})

	RegisterCalculator("Pump.Swap", func(d dex.DEX, logger *zap.Logger) PnLCalculator {
		return &pumpSwapCalculator{dex: d, logger: logger}
	})
}
