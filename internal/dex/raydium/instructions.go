// internal/dex/raydium/instructions.go
package raydium

import (
	"encoding/binary"

	"github.com/gagliardetto/solana-go"
)

// ExecutableSwapInstruction реализует solana.Instruction
type ExecutableSwapInstruction struct {
	programID solana.PublicKey
	accounts  []*solana.AccountMeta
	data      []byte
}

// Реализация интерфейса solana.Instruction
func (ix *ExecutableSwapInstruction) ProgramID() solana.PublicKey {
	return ix.programID
}

func (ix *ExecutableSwapInstruction) Accounts() []*solana.AccountMeta {
	return ix.accounts
}

func (ix *ExecutableSwapInstruction) Data() ([]byte, error) {
	return ix.data, nil
}

// Добавляем вспомогательный конструктор
func NewExecutableSwapInstruction(
	programID solana.PublicKey,
	accounts []*solana.AccountMeta,
	data []byte,
) *ExecutableSwapInstruction {
	return &ExecutableSwapInstruction{
		programID: programID,
		accounts:  accounts,
		data:      data,
	}
}

// buildSwapInstruction создает инструкцию для свапа
func (c *Client) buildSwapInstruction(params *SwapParams) (solana.Instruction, error) {
	accounts := []*solana.AccountMeta{
		solana.Meta(params.Pool.ID).WRITE(),
		solana.Meta(params.Pool.Authority),
		solana.Meta(params.UserWallet).SIGNER(),
		solana.Meta(params.SourceTokenAccount).WRITE(),
		solana.Meta(params.DestinationTokenAccount).WRITE(),
		solana.Meta(params.Pool.BaseVault).WRITE(),
		solana.Meta(params.Pool.QuoteVault).WRITE(),
		solana.Meta(TokenProgramID),
	}

	// Создаем данные инструкции
	data := make([]byte, 17) // 1 + 8 + 8 bytes
	data[0] = 0x02           // swap instruction discriminator

	// Конвертируем SwapDirection в byte
	var dirByte byte
	if params.Direction == SwapDirectionIn {
		dirByte = 1
	} else {
		dirByte = 0
	}
	data[1] = dirByte

	// Записываем AmountIn и MinAmountOut
	binary.LittleEndian.PutUint64(data[2:10], params.AmountIn)
	binary.LittleEndian.PutUint64(data[10:], params.MinAmountOut)

	return NewExecutableSwapInstruction(
		RaydiumV4ProgramID,
		accounts,
		data,
	), nil
}
