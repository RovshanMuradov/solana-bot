// =============================
// File: internal/dex/pumpfun/transactions.go
// =============================
package pumpfun

import (
	"context"
	"fmt"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"go.uber.org/zap"
)

// prepareTransactionContext создает контекст с таймаутом для операции
func (d *DEX) prepareTransactionContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, timeout)
}

// prepareBaseInstructions подготавливает базовые инструкции (приоритет и ATA)
func (d *DEX) prepareBaseInstructions(_ context.Context, priorityFeeSol string, computeUnits uint32) ([]solana.Instruction, solana.PublicKey, error) {
	// Create priority instructions
	priorityInstructions, err := d.priorityManager.CreatePriorityInstructions(priorityFeeSol, computeUnits)
	if err != nil {
		return nil, solana.PublicKey{}, fmt.Errorf("failed to create priority instructions: %w", err)
	}

	// Create Associated Token Account
	userATA, _, err := solana.FindAssociatedTokenAddress(d.wallet.PublicKey, d.config.Mint)
	if err != nil {
		return nil, solana.PublicKey{}, fmt.Errorf("failed to derive associated token account: %w", err)
	}
	ataInstruction := d.wallet.CreateAssociatedTokenAccountIdempotentInstruction(
		d.wallet.PublicKey, d.wallet.PublicKey, d.config.Mint)

	var instructions []solana.Instruction
	instructions = append(instructions, priorityInstructions...)
	instructions = append(instructions, ataInstruction)

	return instructions, userATA, nil
}

// sendAndConfirmTransaction создает, подписывает, отправляет и ожидает подтверждения транзакции
func (d *DEX) sendAndConfirmTransaction(ctx context.Context, instructions []solana.Instruction) (solana.Signature, error) {
	// Get latest blockhash
	blockhash, err := d.client.GetRecentBlockhash(ctx)
	if err != nil {
		return solana.Signature{}, fmt.Errorf("failed to get recent blockhash: %w", err)
	}
	d.logger.Debug("Got blockhash", zap.String("blockhash", blockhash.String()))

	// Create transaction
	tx, err := solana.NewTransaction(
		instructions,
		blockhash,
		solana.TransactionPayer(d.wallet.PublicKey),
	)
	if err != nil {
		return solana.Signature{}, fmt.Errorf("failed to create transaction: %w", err)
	}

	// Sign transaction
	if err := d.wallet.SignTransaction(tx); err != nil {
		return solana.Signature{}, fmt.Errorf("failed to sign transaction: %w", err)
	}

	// Simulate transaction
	simResult, err := d.client.SimulateTransaction(ctx, tx)
	if err != nil || (simResult != nil && simResult.Err != nil) {
		d.logger.Warn("Transaction simulation failed",
			zap.Error(err),
			zap.Any("sim_error", simResult != nil && simResult.Err != nil))
		// Continue anyway as simulation can sometimes fail for valid transactions
	} else {
		d.logger.Info("Transaction simulation successful",
			zap.Uint64("compute_units", simResult.UnitsConsumed))
	}

	// Send transaction
	txSig, err := d.client.SendTransaction(ctx, tx)
	if err != nil {
		return solana.Signature{}, fmt.Errorf("failed to send transaction: %w", err)
	}

	d.logger.Info("Transaction sent successfully",
		zap.String("signature", txSig.String()))

	// Wait for confirmation
	if err := d.client.WaitForTransactionConfirmation(ctx, txSig, rpc.CommitmentConfirmed); err != nil {
		d.logger.Warn("Failed to confirm transaction",
			zap.String("signature", txSig.String()),
			zap.Error(err))
		return txSig, fmt.Errorf("transaction confirmation failed: %w", err)
	}

	d.logger.Info("Transaction confirmed",
		zap.String("signature", txSig.String()))
	return txSig, nil
}
