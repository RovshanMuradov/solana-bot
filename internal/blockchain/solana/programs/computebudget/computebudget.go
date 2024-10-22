// internal/blockchain/solana/programs/computebudget/computebudget.go
package computebudget

import (
	"bytes"
	"encoding/binary"

	"github.com/gagliardetto/solana-go"
)

// ProgramID - публичный ключ программы ComputeBudget
var ProgramID = solana.MustPublicKeyFromBase58("ComputeBudget111111111111111111111111111111")

// ComputeBudgetInstructionTag - тип инструкции для установки цены за вычислительные единицы
const ComputeBudgetInstructionTag uint8 = 0x02

// SetComputeUnitPrice представляет инструкцию для установки цены за вычислительные единицы
type SetComputeUnitPrice struct {
	ComputeUnitPrice uint64
}

// Implement solana.Instruction interface

func (instr *SetComputeUnitPrice) ProgramID() solana.PublicKey {
	return ProgramID
}

func (instr *SetComputeUnitPrice) Accounts() []*solana.AccountMeta {
	return []*solana.AccountMeta{}
}

func (instr *SetComputeUnitPrice) Data() ([]byte, error) {
	buf := new(bytes.Buffer)
	// Write the instruction tag
	if err := binary.Write(buf, binary.LittleEndian, ComputeBudgetInstructionTag); err != nil {
		return nil, err
	}
	// Write the ComputeUnitPrice
	if err := binary.Write(buf, binary.LittleEndian, instr.ComputeUnitPrice); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
