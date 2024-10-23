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

type TransactionBuilder struct {
	instructions []solana.Instruction
	signers      []solana.PrivateKey
	config       computebudget.ComputeBudgetConfig
}

func NewTransactionBuilder() *TransactionBuilder {
	return &TransactionBuilder{
		config: computebudget.NewDefaultConfig(),
	}
}

func (b *TransactionBuilder) SetComputeBudget(units uint32, priceInSol float64) *TransactionBuilder {
	microLamports := computebudget.ConvertSolToMicrolamports(priceInSol)
	b.config = computebudget.ComputeBudgetConfig{
		Units:     units,
		UnitPrice: microLamports,
	}
	return b
}

func (b *TransactionBuilder) AddInstruction(instruction solana.Instruction) *TransactionBuilder {
	b.instructions = append(b.instructions, instruction)
	return b
}

func (b *TransactionBuilder) AddSigner(signer solana.PrivateKey) *TransactionBuilder {
	b.signers = append(b.signers, signer)
	return b
}

func (b *TransactionBuilder) Build(ctx context.Context, client SolanaClient) (*solana.Transaction, error) {
	if len(b.signers) == 0 {
		return nil, fmt.Errorf("no signers provided")
	}

	blockhash, err := client.GetRecentBlockhash(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get recent blockhash: %w", err)
	}

	budgetInstructions, err := computebudget.BuildComputeBudgetInstructions(b.config)
	if err != nil {
		return nil, fmt.Errorf("failed to build compute budget instructions: %w", err)
	}

	allInstructions := append(budgetInstructions, b.instructions...)

	tx, err := solana.NewTransaction(
		allInstructions,
		blockhash,
		solana.TransactionPayer(b.signers[0].PublicKey()),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create transaction: %w", err)
	}

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
