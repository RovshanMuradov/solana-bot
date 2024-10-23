// internal/blockchain/solana/transaction/builder.go
package transaction

import (
	"context"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solana/programs/computebudget"
)

// SolanaClient определяет интерфейс для взаимодействия с Solana
type SolanaClient interface {
	GetRecentBlockhash(ctx context.Context) (solana.Hash, error)
}

// TransactionBuilder помогает конструировать транзакции
type Builder struct {
	instructions []solana.Instruction
	signers      []solana.PrivateKey
	config       computebudget.Config
}

// NewTransactionBuilder создает новый билдер транзакций
func NewTransactionBuilder() *Builder {
	return &Builder{
		config: computebudget.NewDefaultConfig(),
	}
}

// SetComputeBudget устанавливает параметры compute budget
func (b *Builder) SetComputeBudget(units uint32, priceInSol float64) *Builder {
	b.config = computebudget.Config{
		Units:       units,
		PriorityFee: priceInSol,
	}
	return b
}

// AddInstruction добавляет инструкцию в транзакцию
func (b *Builder) AddInstruction(instruction solana.Instruction) *Builder {
	b.instructions = append(b.instructions, instruction)
	return b
}

// AddSigner добавляет подписанта транзакции
func (b *Builder) AddSigner(signer solana.PrivateKey) *Builder {
	b.signers = append(b.signers, signer)
	return b
}

// Build создает и подписывает транзакцию
func (b *Builder) Build(ctx context.Context, client SolanaClient) (*solana.Transaction, error) {
	if len(b.signers) == 0 {
		return nil, fmt.Errorf("no signers provided")
	}

	blockhash, err := client.GetRecentBlockhash(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get recent blockhash: %w", err)
	}

	// Получаем инструкции для compute budget
	budgetInstructions, err := computebudget.BuildInstructions(b.config)
	if err != nil {
		return nil, fmt.Errorf("failed to build compute budget instructions: %w", err)
	}

	// Правильная работа со slice
	instructions := make([]solana.Instruction, 0, len(budgetInstructions)+len(b.instructions))
	instructions = append(instructions, budgetInstructions...)
	instructions = append(instructions, b.instructions...)

	tx, err := solana.NewTransaction(
		instructions,
		blockhash,
		solana.TransactionPayer(b.signers[0].PublicKey()),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create transaction: %w", err)
	}

	// Подписываем транзакцию
	_, err = tx.Sign(func(key solana.PublicKey) *solana.PrivateKey {
		for _, signer := range b.signers {
			if signer.PublicKey().Equals(key) {
				privateCopy := signer
				return &privateCopy
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to sign transaction: %w", err)
	}

	return tx, nil
}
