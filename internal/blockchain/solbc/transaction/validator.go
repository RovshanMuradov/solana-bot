// internal/blockchain/solbc/transaction/validator.go
package transaction

import (
	"github.com/gagliardetto/solana-go"
	"go.uber.org/zap"
)

type Validator struct {
	logger *zap.Logger
}

func NewValidator(logger *zap.Logger) *Validator {
	return &Validator{
		logger: logger.Named("tx-validator"),
	}
}

func (v *Validator) ValidateTransaction(tx *solana.Transaction) error {
	if err := v.ValidateSignatures(tx); err != nil {
		return err
	}

	if err := v.ValidateBlockhash(tx); err != nil {
		return err
	}

	if err := v.ValidateInstructions(tx.Message.Instructions); err != nil {
		return err
	}

	return nil
}

func (v *Validator) ValidateSignatures(tx *solana.Transaction) error {
	if len(tx.Signatures) == 0 {
		return ErrInvalidSignature
	}
	// Дополнительные проверки подписей...
	return nil
}

func (v *Validator) ValidateBlockhash(tx *solana.Transaction) error {
	if tx.Message.RecentBlockhash == (solana.Hash{}) {
		return ErrInvalidBlockhash
	}
	return nil
}

func (v *Validator) ValidateInstructions(instructions []solana.CompiledInstruction) error {
	if len(instructions) == 0 {
		return ErrInvalidInstruction
	}
	// Дополнительные проверки инструкций...
	return nil
}
