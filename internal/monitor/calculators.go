// internal/monitor/calculator.go
package monitor

import (
	"context"
	"fmt"
	"github.com/rovshanmuradov/solana-bot/internal/dex/model"

	"github.com/rovshanmuradov/solana-bot/internal/dex"
	"go.uber.org/zap"
)

// PnLCalculator определяет интерфейс для расчета показателей прибыли и убытка для токенов
type PnLCalculator interface {
	// CalculatePnL вычисляет показатели прибыли и убытка для токенов
	CalculatePnL(ctx context.Context, tokenAmount float64, initialInvestment float64) (*model.PnLResult, error)
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

// init регистрирует калькуляторы при загрузке пакета
func init() {
	RegisterCalculator("Pump.fun", func(d dex.DEX, logger *zap.Logger) PnLCalculator {
		return &pumpFunCalculator{dex: d, logger: logger}
	})

	RegisterCalculator("Pump.Swap", func(d dex.DEX, logger *zap.Logger) PnLCalculator {
		return &pumpSwapCalculator{dex: d, logger: logger}
	})

	RegisterCalculator("Smart DEX", func(d dex.DEX, logger *zap.Logger) PnLCalculator {
		return &smartDEXCalculator{dex: d, logger: logger}
	})
}
