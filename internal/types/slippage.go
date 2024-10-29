// internal/types/slippage.go
package types

import "math"

// SlippageType определяет тип политики проскальзывания
type SlippageType string

const (
	// SlippageFixed использует фиксированное значение minAmountOut
	SlippageFixed SlippageType = "fixed"
	// SlippagePercent использует процент от ожидаемого выхода
	SlippagePercent SlippageType = "percent"
	// SlippageNone не использует ограничение minAmountOut
	SlippageNone SlippageType = "none"
)

// SlippageConfig конфигурирует политику проскальзывания
type SlippageConfig struct {
	// Type определяет тип политики проскальзывания
	Type SlippageType `json:"type"`
	// Value содержит значение для выбранной политики:
	// - для SlippageFixed: точное значение minAmountOut
	// - для SlippagePercent: процент допустимого проскальзывания (например, 1.0 = 1%)
	// - для SlippageNone: игнорируется
	Value float64 `json:"value"`
}

// CalculateMinAmountOut вычисляет minAmountOut на основе политики проскальзывания
func CalculateMinAmountOut(expectedAmount float64, config SlippageConfig) uint64 {
	switch config.Type {
	case SlippageFixed:
		return uint64(config.Value)
	case SlippagePercent:
		// Вычисляем минимальный выход с учетом процента проскальзывания
		// Например, если проскальзывание 1% (value = 1.0), то минимум будет 99% от ожидаемого
		multiplier := 1.0 - (config.Value / 100.0)
		return uint64(math.Floor(expectedAmount * multiplier))
	case SlippageNone:
		// Возвращаем 1 как минимальное значение для прохождения валидации
		return 1
	default:
		// По умолчанию используем максимально гибкий вариант
		return 1
	}
}
