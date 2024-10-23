// internal/types/priority.go
package types

import (
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solana/programs/computebudget"
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

// PriorityConfig содержит настройки для приоритета транзакции
type PriorityConfig struct {
	ComputeUnits uint32  // Количество compute units
	PriorityFee  float64 // Приоритетная комиссия в SOL
	HeapSize     uint32  // Дополнительная heap память (опционально)
}

// PriorityManager управляет приоритетами транзакций
type PriorityManager struct {
	profiles map[PriorityLevel]*PriorityConfig
	logger   *zap.Logger
}

// NewPriorityManager создает менеджер с предустановленными профилями
func NewPriorityManager(logger *zap.Logger) *PriorityManager {
	return &PriorityManager{
		profiles: map[PriorityLevel]*PriorityConfig{
			PriorityLow: {
				ComputeUnits: computebudget.DefaultUnits,
				PriorityFee:  0.000001, // 1 микро SOL
			},
			PriorityMedium: {
				ComputeUnits: computebudget.StandardUnits,
				PriorityFee:  0.000005, // 5 микро SOL
			},
			PriorityHigh: {
				ComputeUnits: 800_000,
				PriorityFee:  0.00001, // 10 микро SOL
			},
			PriorityExtreme: {
				ComputeUnits: computebudget.SnipingUnits,
				PriorityFee:  0.00005,   // 50 микро SOL
				HeapSize:     32 * 1024, // 32KB для сложных операций
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

	budgetConfig := computebudget.Config{
		Units:         config.ComputeUnits,
		PriorityFee:   config.PriorityFee,
		HeapFrameSize: config.HeapSize,
	}

	return computebudget.BuildInstructions(budgetConfig)
}

// GetPriorityLevel определяет уровень приоритета на основе комиссии
func GetPriorityLevel(priorityFee float64) PriorityLevel {
	switch {
	case priorityFee >= 0.00005:
		return PriorityExtreme
	case priorityFee >= 0.00001:
		return PriorityHigh
	case priorityFee >= 0.000005:
		return PriorityMedium
	default:
		return PriorityLow
	}
}

// CreateCustomPriorityInstructions создает инструкции с пользовательскими настройками
func (pm *PriorityManager) CreateCustomPriorityInstructions(priorityFee float64, units uint32) ([]solana.Instruction, error) {
	if priorityFee < 0 {
		return nil, fmt.Errorf("priority fee cannot be negative: %f", priorityFee)
	}

	budgetConfig := computebudget.Config{
		Units:       units,
		PriorityFee: priorityFee,
	}

	return computebudget.BuildInstructions(budgetConfig)
}

// GetProfile возвращает конфигурацию профиля по уровню приоритета
func (pm *PriorityManager) GetProfile(level PriorityLevel) (*PriorityConfig, error) {
	config, ok := pm.profiles[level]
	if !ok {
		return nil, fmt.Errorf("unknown priority level: %s", level)
	}
	return config, nil
}
