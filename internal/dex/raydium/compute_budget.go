// internal/dex/raydium/compute_budget.go
package raydium

import (
	"github.com/gagliardetto/solana-go"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solana/programs/computebudget"
	"go.uber.org/zap"
)

// createComputeBudgetInstructions создает инструкции для настройки бюджета
func createComputeBudgetInstructions(priorityFee float64, units uint32, logger *zap.Logger) ([]solana.Instruction, error) {
	config := computebudget.ComputeBudgetConfig{
		Units:     units,
		UnitPrice: computebudget.ConvertSolToMicrolamports(priorityFee),
		Priority:  getPriorityLevel(priorityFee),
	}

	return computebudget.BuildComputeBudgetInstructions(config)
}

// getPriorityLevel определяет уровень приоритета на основе комиссии
func getPriorityLevel(priorityFee float64) computebudget.PriorityLevel {
	switch {
	case priorityFee >= 0.00005:
		return computebudget.PriorityExtreme
	case priorityFee >= 0.00001:
		return computebudget.PriorityHigh
	case priorityFee >= 0.000005:
		return computebudget.PriorityMedium
	default:
		return computebudget.PriorityLow
	}
}
