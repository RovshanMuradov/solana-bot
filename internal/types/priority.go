package types

import (
	"fmt"

	"github.com/gagliardetto/solana-go"
	computebudget "github.com/gagliardetto/solana-go/programs/compute-budget"
	"go.uber.org/zap"
)

// PriorityManager handles transaction priority fee calculation and formatting
type PriorityManager struct {
	logger *zap.Logger
}

// Conversion constants
const (
	LamportsPerSol      = 1_000_000_000     // 1 SOL = 10^9 lamports
	MicroLamportsPerSol = 1_000_000_000_000 // 1 SOL = 10^12 micro-lamports
	DefaultPriorityFee  = 5_000             // Default priority fee in micro-lamports
	DefaultComputeUnits = 200_000           // Default compute units
	DefaultHeapSize     = 0                 // Default additional heap size
)

// NewPriorityManager creates a new priority manager
func NewPriorityManager(logger *zap.Logger) *PriorityManager {
	return &PriorityManager{
		logger: logger,
	}
}

// SolToLamports converts SOL to lamports
func (pm *PriorityManager) SolToLamports(sol float64) uint64 {
	return uint64(sol * LamportsPerSol)
}

// LamportsToSol converts lamports to SOL
func (pm *PriorityManager) LamportsToSol(lamports uint64) float64 {
	return float64(lamports) / LamportsPerSol
}

// SolToMicroLamports converts SOL to micro-lamports
func (pm *PriorityManager) SolToMicroLamports(sol float64) uint64 {
	return uint64(sol * MicroLamportsPerSol)
}

// MicroLamportsToSol converts micro-lamports to SOL
func (pm *PriorityManager) MicroLamportsToSol(microLamports uint64) float64 {
	return float64(microLamports) / MicroLamportsPerSol
}

// FormatLamports formats lamports as SOL with proper decimal places
func (pm *PriorityManager) FormatLamports(lamports uint64) string {
	return fmt.Sprintf("%.9f SOL", pm.LamportsToSol(lamports))
}

// FormatMicroLamports formats micro-lamports as SOL with proper decimal places
func (pm *PriorityManager) FormatMicroLamports(microLamports uint64) string {
	return fmt.Sprintf("%.12f SOL", pm.MicroLamportsToSol(microLamports))
}

// CreatePriorityInstructions creates compute budget instructions with custom priority fee
// If priorityFeeSol is "default", it uses the default fee value
func (pm *PriorityManager) CreatePriorityInstructions(priorityFeeSol string, computeUnits uint32) ([]solana.Instruction, error) {
	var priorityFee uint64

	// Handle default value
	if priorityFeeSol == "default" {
		priorityFee = DefaultPriorityFee
		pm.logger.Debug("Using default priority fee",
			zap.Uint64("micro_lamports", priorityFee),
			zap.String("sol", pm.FormatMicroLamports(priorityFee)))
	} else {
		// Parse SOL value from string and convert to micro-lamports
		var solValue float64
		_, err := fmt.Sscanf(priorityFeeSol, "%f", &solValue)
		if err != nil {
			return nil, fmt.Errorf("invalid priority fee format: %w", err)
		}

		priorityFee = pm.SolToMicroLamports(solValue)
		pm.logger.Debug("Custom priority fee",
			zap.Float64("sol_input", solValue),
			zap.Uint64("micro_lamports", priorityFee))
	}

	// Use default compute units if not specified
	if computeUnits == 0 {
		computeUnits = DefaultComputeUnits
	}

	return pm.createInstructions(priorityFee, computeUnits, DefaultHeapSize)
}

// createInstructions creates compute budget instructions with the specified parameters
func (pm *PriorityManager) createInstructions(priorityFee uint64, computeUnits uint32, heapSize uint32) ([]solana.Instruction, error) {
	var instructions []solana.Instruction

	// Set compute unit limit
	if computeUnits > 0 {
		inst := computebudget.NewSetComputeUnitLimitInstruction(computeUnits).Build()
		instructions = append(instructions, inst)
		pm.logger.Debug("Added compute unit limit instruction", zap.Uint32("units", computeUnits))
	}

	// Set compute unit price (priority fee)
	if priorityFee > 0 {
		inst := computebudget.NewSetComputeUnitPriceInstruction(priorityFee).Build()
		instructions = append(instructions, inst)
		pm.logger.Debug("Added priority fee instruction",
			zap.Uint64("micro_lamports", priorityFee),
			zap.String("sol", pm.FormatMicroLamports(priorityFee)))
	}

	// Request heap frame (if needed)
	if heapSize > 0 {
		inst := computebudget.NewRequestHeapFrameInstruction(heapSize).Build()
		instructions = append(instructions, inst)
		pm.logger.Debug("Added heap frame instruction", zap.Uint32("size", heapSize))
	}

	return instructions, nil
}
