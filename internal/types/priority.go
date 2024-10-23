// internal/types/priority.go
package types

import (
	"fmt"

	"github.com/gagliardetto/solana-go"
	"go.uber.org/zap"
)

// PriorityLevel представляет уровень приоритета транзакции
type PriorityLevel string

const (
	PriorityLow     PriorityLevel = "low"     // Для обычных транзакций
	PriorityMedium  PriorityLevel = "medium"  // Для важных транзакций
	PriorityHigh    PriorityLevel = "high"    // Для срочных транзакций
	PriorityExtreme PriorityLevel = "extreme" // Для снайпинга при высокой конкуренции
)

// PriorityProfile содержит настройки для разных сценариев использования
type PriorityProfile struct {
	Name        string        // Название профиля
	Description string        // Описание использования
	Priority    PriorityLevel // Уровень приоритета
	Units       uint32        // Количество compute units
}

// PriorityManager управляет приоритетами транзакций
type PriorityManager struct {
	profiles map[PriorityLevel]*PriorityConfig
	logger   *zap.Logger
}

type PriorityConfig struct {
	ComputeUnits uint32
	PriorityFee  float64
}

// NewPriorityManager создает менеджер с предустановленными профилями
func NewPriorityManager(logger *zap.Logger) *PriorityManager {
	return &PriorityManager{
		profiles: map[PriorityLevel]*PriorityConfig{
			PriorityLow: {
				ComputeUnits: 200_000,
				PriorityFee:  0.000001, // 1 микро SOL
			},
			PriorityMedium: {
				ComputeUnits: 400_000,
				PriorityFee:  0.000005, // 5 микро SOL
			},
			PriorityHigh: {
				ComputeUnits: 800_000,
				PriorityFee:  0.00001, // 10 микро SOL
			},
			PriorityExtreme: {
				ComputeUnits: 1_000_000,
				PriorityFee:  0.00005, // 50 микро SOL
			},
		},
		logger: logger,
	}
}

// GetPriorityInstructions создает инструкции на основе выбранного профиля
func (pm *PriorityManager) GetPriorityInstructions(level PriorityLevel) ([]solana.Instruction, error) {
	config, ok := pm.profiles[level]
	if !ok {
		return nil, fmt.Errorf("unknown priority level: %s", level)
	}

	return createPriorityInstructions(config.ComputeUnits, config.PriorityFee)
}

// Вспомогательные функции
func createPriorityInstructions(units uint32, priorityFee float64) ([]solana.Instruction, error) {
	// ... реализация создания инструкций ...
}
