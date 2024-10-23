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

// Предопределенные размеры compute units
const (
	DefaultUnits  uint32 = 200_000
	SnipingUnits  uint32 = 1_000_000
	StandardUnits uint32 = 400_000
)

// ComputeBudgetConfig содержит настройки для транзакции
// Переименовываем ComputeBudgetConfig -> Config
type Config struct {
	Units         uint32  // Количество compute units
	PriorityFee   float64 // Приоритетная комиссия в SOL
	HeapFrameSize uint32  // Дополнительная heap память (опционально)
}

// NewDefaultConfig создает конфигурацию по умолчанию
func NewDefaultConfig() Config {
	return Config{
		Units:       DefaultUnits,
		PriorityFee: 0.000001, // 1 микро SOL
	}
}

// NewSnipingConfig создает конфигурацию для снайпинга
func NewSnipingConfig() Config {
	return Config{
		Units:         SnipingUnits,
		PriorityFee:   0.00005,   // 50 микро SOL
		HeapFrameSize: 32 * 1024, // 32KB для сложных операций
	}
}

// ConvertSolToMicrolamports конвертирует SOL в микроламппорты
func ConvertSolToMicrolamports(sol float64) uint64 {
	return uint64(sol * 1e9)
}

// BuildInstructions создает все необходимые инструкции для настройки compute budget
func BuildInstructions(config Config) ([]solana.Instruction, error) {
	var instructions []solana.Instruction

	// 1. Инструкция для установки лимита compute units
	if config.Units > 0 {
		limitInstr, err := createSetComputeUnitLimitInstruction(config.Units)
		if err != nil {
			return nil, fmt.Errorf("failed to create unit limit instruction: %w", err)
		}
		instructions = append(instructions, limitInstr)
	}

	// 2. Инструкция для установки приоритетной комиссии
	if config.PriorityFee > 0 {
		priceInstr, err := createSetComputeUnitPriceInstruction(config.PriorityFee)
		if err != nil {
			return nil, fmt.Errorf("failed to create unit price instruction: %w", err)
		}
		instructions = append(instructions, priceInstr)
	}

	// 3. Инструкция для выделения дополнительной heap памяти
	if config.HeapFrameSize > 0 {
		heapInstr, err := createRequestHeapFrameInstruction(config.HeapFrameSize)
		if err != nil {
			return nil, fmt.Errorf("failed to create heap frame instruction: %w", err)
		}
		instructions = append(instructions, heapInstr)
	}

	return instructions, nil
}

func createSetComputeUnitLimitInstruction(units uint32) (solana.Instruction, error) {
	buf := new(bytes.Buffer)
	if err := binary.Write(buf, binary.LittleEndian, SetComputeUnitLimit); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, units); err != nil {
		return nil, err
	}
	return solana.NewInstruction(ProgramID, []*solana.AccountMeta{}, buf.Bytes()), nil
}

func createSetComputeUnitPriceInstruction(priorityFee float64) (solana.Instruction, error) {
	buf := new(bytes.Buffer)
	if err := binary.Write(buf, binary.LittleEndian, SetComputeUnitPrice); err != nil {
		return nil, err
	}
	microLamports := ConvertSolToMicrolamports(priorityFee)
	if err := binary.Write(buf, binary.LittleEndian, microLamports); err != nil {
		return nil, err
	}
	return solana.NewInstruction(ProgramID, []*solana.AccountMeta{}, buf.Bytes()), nil
}

func createRequestHeapFrameInstruction(heapSize uint32) (solana.Instruction, error) {
	buf := new(bytes.Buffer)
	if err := binary.Write(buf, binary.LittleEndian, RequestHeapFrame); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, heapSize); err != nil {
		return nil, err
	}
	return solana.NewInstruction(ProgramID, []*solana.AccountMeta{}, buf.Bytes()), nil
}
