// internal/types/slippage.go
package types

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

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
	Type  SlippageType `json:"type"`
	Value float64      `json:"value"`
}

// NewSlippageConfig создает новую конфигурацию слиппажа из строки
func NewSlippageConfig(value string) (SlippageConfig, error) {
	// Если значение пустое, используем процентный слиппаж 1%
	if value == "" {
		return SlippageConfig{
			Type:  SlippagePercent,
			Value: 1.0,
		}, nil
	}

	// Проверяем специальные значения
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "none" {
		return SlippageConfig{
			Type:  SlippageNone,
			Value: 0,
		}, nil
	}

	// Проверяем фиксированное значение
	if strings.HasPrefix(value, "fixed:") {
		fixedValue, err := strconv.ParseFloat(strings.TrimPrefix(value, "fixed:"), 64)
		if err != nil {
			return SlippageConfig{}, fmt.Errorf("invalid fixed slippage value: %w", err)
		}
		if fixedValue <= 0 {
			return SlippageConfig{}, fmt.Errorf("fixed slippage value must be positive")
		}
		return SlippageConfig{
			Type:  SlippageFixed,
			Value: fixedValue,
		}, nil
	}

	// Парсим процентное значение
	percentValue, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return SlippageConfig{}, fmt.Errorf("invalid slippage percentage: %w", err)
	}
	if percentValue <= 0 || percentValue > 100 {
		return SlippageConfig{}, fmt.Errorf("slippage percentage must be between 0 and 100")
	}

	return SlippageConfig{
		Type:  SlippagePercent,
		Value: percentValue,
	}, nil
}

// Validate проверяет корректность конфигурации
func (c SlippageConfig) Validate() error {
	switch c.Type {
	case SlippageFixed:
		if c.Value <= 0 {
			return fmt.Errorf("fixed slippage value must be positive")
		}
	case SlippagePercent:
		if c.Value <= 0 || c.Value > 100 {
			return fmt.Errorf("slippage percentage must be between 0 and 100")
		}
	case SlippageNone:
		// Для SlippageNone значение не важно
	default:
		return fmt.Errorf("invalid slippage type: %s", c.Type)
	}
	return nil
}

// CalculateMinAmountOut вычисляет minAmountOut на основе политики проскальзывания
func CalculateMinAmountOut(expectedAmount float64, config SlippageConfig) uint64 {
	if err := config.Validate(); err != nil {
		// В случае ошибки возвращаем безопасное значение
		return 1
	}

	switch config.Type {
	case SlippageFixed:
		return uint64(config.Value)
	case SlippagePercent:
		// Вычисляем минимальный выход с учетом процента проскальзывания
		multiplier := 1.0 - (config.Value / 100.0)
		return uint64(math.Floor(expectedAmount * multiplier))
	case SlippageNone:
		return 1
	default:
		return 1
	}
}

// String возвращает строковое представление конфигурации
func (c SlippageConfig) String() string {
	switch c.Type {
	case SlippageFixed:
		return fmt.Sprintf("fixed:%.6f", c.Value)
	case SlippagePercent:
		return fmt.Sprintf("%.2f%%", c.Value)
	case SlippageNone:
		return "none"
	default:
		return "invalid"
	}
}
