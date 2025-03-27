// =============================
// File: internal/dex/pumpswap/errors.go
// =============================
package pumpswap

import (
	"fmt"
	"go.uber.org/zap"
	"strconv"
	"strings"
)

// Константы для кодов ошибок Solana
const (
	SlippageExceededCode    = "0x1774"
	SlippageExceededCodeInt = 6004
)

// SlippageExceededError представляет ошибку превышения проскальзывания
type SlippageExceededError struct {
	SlippagePercent float64
	Amount          uint64
	OriginalError   error
}

// IsSlippageExceededError определяет, является ли ошибка ошибкой превышения проскальзывания
func IsSlippageExceededError(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "ExceededSlippage") ||
		strings.Contains(err.Error(), SlippageExceededCode) ||
		strings.Contains(err.Error(), strconv.Itoa(SlippageExceededCodeInt)))
}

func (e *SlippageExceededError) Error() string {
	return fmt.Sprintf("slippage exceeded: transaction requires more funds than maximum specified (%f%%): %v",
		e.SlippagePercent, e.OriginalError)
}

func (e *SlippageExceededError) Unwrap() error {
	return e.OriginalError
}

// handleSwapError обрабатывает специфичные ошибки операции свапа
func (d *DEX) handleSwapError(err error, params SwapParams) error {
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
