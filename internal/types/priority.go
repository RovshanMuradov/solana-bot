package types

import (
	"fmt"

	"github.com/gagliardetto/solana-go"
	computebudget "github.com/gagliardetto/solana-go/programs/compute-budget"
	"go.uber.org/zap"
)

type PriorityLevel string

const (
	PriorityLow     PriorityLevel = "low"
	PriorityMedium  PriorityLevel = "medium"
	PriorityHigh    PriorityLevel = "high"
	PriorityExtreme PriorityLevel = "extreme"
)

type PriorityConfig struct {
	ComputeUnits uint32 // Number of compute units
	PriorityFee  uint64 // Priority fee in micro-lamports
	HeapSize     uint32 // Additional heap memory (optional)
}

type PriorityManager struct {
	profiles map[PriorityLevel]*PriorityConfig
	logger   *zap.Logger
}

func NewPriorityManager(logger *zap.Logger) *PriorityManager {
	return &PriorityManager{
		profiles: map[PriorityLevel]*PriorityConfig{
			PriorityLow: {
				ComputeUnits: 200_000,
				PriorityFee:  1_000, // 0.000001 SOL in micro-lamports
			},
			PriorityMedium: {
				ComputeUnits: 400_000,
				PriorityFee:  5_000, // 0.000005 SOL in micro-lamports
			},
			PriorityHigh: {
				ComputeUnits: 800_000,
				PriorityFee:  10_000, // 0.00001 SOL in micro-lamports
			},
			PriorityExtreme: {
				ComputeUnits: 1_000_000,
				PriorityFee:  50_000,    // 0.00005 SOL in micro-lamports
				HeapSize:     32 * 1024, // 32KB
			},
		},
		logger: logger,
	}
}

func (pm *PriorityManager) CreatePriorityInstructions(level PriorityLevel) ([]solana.Instruction, error) {
	config, ok := pm.profiles[level]
	if !ok {
		return nil, fmt.Errorf("unknown priority level: %s", level)
	}

	return pm.createInstructions(config)
}

func (pm *PriorityManager) CreateCustomPriorityInstructions(priorityFee uint64, units uint32) ([]solana.Instruction, error) {
	// Remove unnecessary check since priorityFee is unsigned
	config := &PriorityConfig{
		ComputeUnits: units,
		PriorityFee:  priorityFee,
	}

	return pm.createInstructions(config)
}

func (pm *PriorityManager) createInstructions(config *PriorityConfig) ([]solana.Instruction, error) {
	var instructions []solana.Instruction

	// Set compute unit limit
	if config.ComputeUnits > 0 {
		inst := computebudget.NewSetComputeUnitLimitInstruction(config.ComputeUnits).Build()
		instructions = append(instructions, inst)
	}

	// Set compute unit price
	if config.PriorityFee > 0 {
		inst := computebudget.NewSetComputeUnitPriceInstruction(config.PriorityFee).Build()
		instructions = append(instructions, inst)
	}

	// Request heap frame
	if config.HeapSize > 0 {
		inst := computebudget.NewRequestHeapFrameInstruction(config.HeapSize).Build()
		instructions = append(instructions, inst)
	}

	return instructions, nil
}
