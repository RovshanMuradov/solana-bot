// internal/blockchain/solana/programs/computebudget/computebudget.go
package computebudget

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/gagliardetto/solana-go"
)

var ProgramID = solana.MustPublicKeyFromBase58("ComputeBudget111111111111111111111111111111")

const (
	RequestUnitsDeprecated uint8 = 0
	RequestHeapFrame       uint8 = 1
	SetComputeUnitLimit    uint8 = 2
	SetComputeUnitPrice    uint8 = 3
)

// Структуры инструкций
type SetComputeUnitLimitInstruction struct {
	Units uint32
}

type SetComputeUnitPriceInstruction struct {
	MicroLamports uint64
}

// Предопределенные профили
const (
	DefaultUnits  uint32 = 200_000
	SnipingUnits  uint32 = 1_000_000
	StandardUnits uint32 = 400_000
)

// PriorityLevel определяет уровень приоритета транзакции
type PriorityLevel string

const (
	PriorityLow     PriorityLevel = "low"
	PriorityMedium  PriorityLevel = "medium"
	PriorityHigh    PriorityLevel = "high"
	PriorityExtreme PriorityLevel = "extreme"
)

// ComputeBudgetConfig содержит конфигурацию для транзакции
type ComputeBudgetConfig struct {
	Units     uint32
	UnitPrice uint64
	Priority  PriorityLevel
}

// NewDefaultConfig создает конфигурацию по умолчанию
func NewDefaultConfig() ComputeBudgetConfig {
	return ComputeBudgetConfig{
		Units:     DefaultUnits,
		UnitPrice: ConvertSolToMicrolamports(0.000001),
		Priority:  PriorityLow,
	}
}

// NewSnipingConfig создает конфигурацию для снайпинга
func NewSnipingConfig() ComputeBudgetConfig {
	return ComputeBudgetConfig{
		Units:     SnipingUnits,
		UnitPrice: ConvertSolToMicrolamports(0.00005),
		Priority:  PriorityExtreme,
	}
}

// ConvertSolToMicrolamports конвертирует SOL в микроламппорты
func ConvertSolToMicrolamports(sol float64) uint64 {
	return uint64(sol * 1e15)
}

// BuildComputeBudgetInstructions создает инструкции для настройки бюджета
func BuildComputeBudgetInstructions(config ComputeBudgetConfig) ([]solana.Instruction, error) {
	if config.Units == 0 {
		config = NewDefaultConfig()
	}

	var instructions []solana.Instruction

	limitInstruction, err := (&SetComputeUnitLimitInstruction{
		Units: config.Units,
	}).Build()
	if err != nil {
		return nil, fmt.Errorf("failed to build compute unit limit instruction: %w", err)
	}
	instructions = append(instructions, limitInstruction)

	if config.UnitPrice > 0 {
		priceInstruction, err := (&SetComputeUnitPriceInstruction{
			MicroLamports: config.UnitPrice,
		}).Build()
		if err != nil {
			return nil, fmt.Errorf("failed to build compute unit price instruction: %w", err)
		}
		instructions = append(instructions, priceInstruction)
	}

	return instructions, nil
}

// Build создает инструкцию для установки лимита compute units
func (instr *SetComputeUnitLimitInstruction) Build() (solana.Instruction, error) {
	buf := new(bytes.Buffer)
	if err := binary.Write(buf, binary.LittleEndian, SetComputeUnitLimit); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, instr.Units); err != nil {
		return nil, err
	}
	return solana.NewInstruction(
		ProgramID,
		[]*solana.AccountMeta{},
		buf.Bytes(),
	), nil
}

// Build создает инструкцию для установки цены compute units
func (instr *SetComputeUnitPriceInstruction) Build() (solana.Instruction, error) {
	buf := new(bytes.Buffer)
	if err := binary.Write(buf, binary.LittleEndian, SetComputeUnitPrice); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, instr.MicroLamports); err != nil {
		return nil, err
	}
	return solana.NewInstruction(
		ProgramID,
		[]*solana.AccountMeta{},
		buf.Bytes(),
	), nil
}
