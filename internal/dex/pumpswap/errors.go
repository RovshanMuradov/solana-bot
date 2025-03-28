// =============================
// File: internal/dex/pumpswap/errors.go
// =============================
package pumpswap

import (
	"errors"
	"fmt"
	"go.uber.org/zap"
	"strings"
)

// Константы для кодов ошибок Solana
const SlippageExceededErrorCode = 6004

// ErrSlippageExceeded - сентинельная ошибка для проверки через errors.Is
var ErrSlippageExceeded = errors.New("slippage exceeded")

// SlippageExceededError представляет ошибку превышения проскальзывания
type SlippageExceededError struct {
	SlippagePercent float64
	Amount          uint64
	OriginalError   error
}

func (e *SlippageExceededError) Error() string {
	return fmt.Sprintf("slippage exceeded: transaction requires more funds than maximum specified (%.2f%%): %v",
		e.SlippagePercent, e.OriginalError)
}

func (e *SlippageExceededError) Unwrap() error {
	return e.OriginalError
}

// Is позволяет использовать errors.Is для проверки типа ошибки
func (e *SlippageExceededError) Is(target error) bool {
	return target == ErrSlippageExceeded
}

// IsSlippageExceededError определяет, является ли ошибка ошибкой превышения проскальзывания
func IsSlippageExceededError(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, ErrSlippageExceeded) {
		return true
	}

	// Для обратной совместимости проверяем строковое представление
	errStr := err.Error()
	return strings.Contains(errStr, "ExceededSlippage") ||
		strings.Contains(errStr, "0x1774") ||
		strings.Contains(errStr, fmt.Sprintf("%d", SlippageExceededErrorCode))
}

// handleSwapError обрабатывает специфичные ошибки операции свапа
func (d *DEX) handleSwapError(err error, params SwapParams) error {
	// Проверяем, не является ли уже ошибка типа SlippageExceededError
	var slippageErr *SlippageExceededError
	if errors.As(err, &slippageErr) {
		d.logger.Warn("Slippage error already handled", zap.Error(err))
		return err
	}

	if IsSlippageExceededError(err) {
		d.logger.Warn("Exceeded slippage error - try increasing slippage percentage",
			zap.Float64("current_slippage_percent", params.SlippagePercent),
			zap.Uint64("amount", params.Amount),
			zap.Error(err))
		return &SlippageExceededError{
			SlippagePercent: params.SlippagePercent,
			Amount:          params.Amount,
			OriginalError:   err,
		}
	}
	return err
}
