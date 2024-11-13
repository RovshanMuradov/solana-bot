// internal/dex/raydium/instructions.go
package raydium

import (
	"context"
	"encoding/binary"
	"fmt"

	"github.com/gagliardetto/solana-go"
	computebudget "github.com/gagliardetto/solana-go/programs/compute-budget"
	"go.uber.org/zap"
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

// Добавить методы:

// PrepareSwapInstructions подготавливает все инструкции для свапа
func (c *Client) PrepareSwapInstructions(params *SwapParams) ([]solana.Instruction, error) {
	if params == nil {
		return nil, fmt.Errorf("swap params cannot be nil")
	}

	c.logger.Debug("preparing swap instructions",
		zap.String("user_wallet", params.UserWallet.String()),
		zap.String("pool_id", params.Pool.ID.String()),
		zap.Uint64("amount_in", params.AmountIn))

	instructions := make([]solana.Instruction, 0)

	// 1. Добавляем инструкцию для установки compute budget
	computeLimitIx := computebudget.NewSetComputeUnitLimitInstructionBuilder().
		SetUnits(MaxComputeUnitLimit).
		Build()
	instructions = append(instructions, computeLimitIx)

	// 2. Добавляем инструкцию для установки приоритетной комиссии
	if params.PriorityFeeLamports > 0 {
		computePriceIx := computebudget.NewSetComputeUnitPriceInstructionBuilder().
			SetMicroLamports(params.PriorityFeeLamports).
			Build()
		instructions = append(instructions, computePriceIx)
	}

	// 3. Проверяем необходимость создания токен-аккаунтов
	sourceExists, err := c.checkTokenAccount(context.Background(), params.SourceTokenAccount)
	if err != nil {
		return nil, fmt.Errorf("failed to check source token account: %w", err)
	}

	destExists, err := c.checkTokenAccount(context.Background(), params.DestinationTokenAccount)
	if err != nil {
		return nil, fmt.Errorf("failed to check destination token account: %w", err)
	}

	// 4. Добавляем инструкции создания токен-аккаунтов если необходимо
	if !sourceExists {
		createSourceATAIx, err := createAssociatedTokenAccountInstruction(
			params.UserWallet,
			params.UserWallet,
			params.Pool.BaseMint,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create source ATA instruction: %w", err)
		}
		instructions = append(instructions, createSourceATAIx)

		c.logger.Debug("adding create source ATA instruction",
			zap.String("mint", params.Pool.BaseMint.String()))
	}

	if !destExists {
		createDestATAIx, err := createAssociatedTokenAccountInstruction(
			params.UserWallet,
			params.UserWallet,
			params.Pool.QuoteMint,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create destination ATA instruction: %w", err)
		}
		instructions = append(instructions, createDestATAIx)

		c.logger.Debug("adding create destination ATA instruction",
			zap.String("mint", params.Pool.QuoteMint.String()))
	}

	// 5. Определяем направление свапа
	swapDirection, err := c.DetermineSwapDirection(
		params.Pool,
		params.Pool.BaseMint,
		params.Pool.QuoteMint,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to determine swap direction: %w", err)
	}
	params.Direction = swapDirection

	// 6. Создаем основную инструкцию свапа
	swapAccounts := []*solana.AccountMeta{
		solana.Meta(params.Pool.ID).WRITE(),                 // Pool
		solana.Meta(params.Pool.Authority),                  // Pool Authority
		solana.Meta(params.UserWallet).SIGNER(),             // User wallet
		solana.Meta(params.SourceTokenAccount).WRITE(),      // Source token account
		solana.Meta(params.DestinationTokenAccount).WRITE(), // Destination token account
		solana.Meta(params.Pool.BaseVault).WRITE(),          // Base vault
		solana.Meta(params.Pool.QuoteVault).WRITE(),         // Quote vault
		solana.Meta(TokenProgramID),                         // Token program
	}

	// Создаем данные инструкции свапа
	swapData := make([]byte, 17) // 1 + 8 + 8 bytes
	swapData[0] = 0x02           // swap instruction discriminator
	swapData[1] = byte(params.Direction)
	binary.LittleEndian.PutUint64(swapData[2:10], params.AmountIn)
	binary.LittleEndian.PutUint64(swapData[10:], params.MinAmountOut)

	swapIx := NewExecutableSwapInstruction(
		RaydiumV4ProgramID,
		swapAccounts,
		swapData,
	)
	instructions = append(instructions, swapIx)

	// Логируем итоговый набор инструкций
	c.logger.Info("swap instructions prepared",
		zap.Int("total_instructions", len(instructions)),
		zap.Bool("includes_ata_creation", !sourceExists || !destExists),
		zap.String("direction", fmt.Sprintf("%d", swapDirection)),
		zap.Uint64("amount_in", params.AmountIn),
		zap.Uint64("min_amount_out", params.MinAmountOut))

	return instructions, nil
}

// createAssociatedTokenAccountInstruction создает инструкцию для создания ATA
func createAssociatedTokenAccountInstruction(
	payer solana.PublicKey,
	owner solana.PublicKey,
	mint solana.PublicKey,
) (solana.Instruction, error) {
	// Находим адрес ATA
	ata, _, err := solana.FindAssociatedTokenAddress(
		owner,
		mint,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to find ATA address: %w", err)
	}

	// Создаем инструкцию с правильным списком аккаунтов
	accounts := []*solana.AccountMeta{
		solana.Meta(payer).SIGNER().WRITE(), // Плательщик комиссии
		solana.Meta(ata).WRITE(),            // Создаваемый ATA аккаунт
		solana.Meta(owner),                  // Владелец
		solana.Meta(mint),                   // Mint токена
		solana.Meta(SystemProgramID),        // System Program
		solana.Meta(TokenProgramID),         // Token Program
		solana.Meta(SysvarRentPubkey),       // Sysvar Rent
	}

	// Создаем данные инструкции (для создания ATA не требуются дополнительные данные)
	data := []byte{1} // Discriminator для создания ATA

	return &ExecutableSwapInstruction{
		programID: TokenProgramID,
		accounts:  accounts,
		data:      data,
	}, nil
}
