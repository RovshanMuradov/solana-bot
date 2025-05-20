// =============================
// File: internal/dex/pumpfun/transactions.go
// =============================
package pumpfun

import (
	"context"
	"fmt"
	computebudget "github.com/gagliardetto/solana-go/programs/compute-budget"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain"
	"go.uber.org/zap"
)

// prepareTransactionContext создает контекст с таймаутом для операции.
func (d *DEX) prepareTransactionContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, timeout)
}

// prepareBaseInstructions подготавливает базовые инструкции для транзакции.
func (d *DEX) prepareBaseInstructions(_ context.Context, priorityFeeSol string, computeUnits uint32) ([]solana.Instruction, solana.PublicKey, error) {
	var instructions []solana.Instruction

	// Set compute unit limit
	if computeUnits == 0 {
		computeUnits = 200_000 // Default compute units
	}
	instructions = append(instructions, computebudget.NewSetComputeUnitLimitInstruction(computeUnits).Build())

	// Handle priority fee
	var priorityFee uint64
	if priorityFeeSol == "default" {
		priorityFee = 5_000 // Default priority fee (5000 micro-lamports)
	} else {
		var solValue float64
		if _, err := fmt.Sscanf(priorityFeeSol, "%f", &solValue); err != nil {
			return nil, solana.PublicKey{}, fmt.Errorf("invalid priority fee format: %w", err)
		}
		priorityFee = uint64(solValue * 1_000_000_000_000) // SOL to micro-lamports
	}

	instructions = append(instructions, computebudget.NewSetComputeUnitPriceInstruction(priorityFee).Build())

	// Create ATA instruction
	userATA, _, err := solana.FindAssociatedTokenAddress(d.wallet.PublicKey, d.config.Mint)
	if err != nil {
		return nil, solana.PublicKey{}, fmt.Errorf("failed to derive associated token account: %w", err)
	}

	ataInstruction := d.wallet.CreateAssociatedTokenAccountIdempotentInstruction(
		d.wallet.PublicKey, d.wallet.PublicKey, d.config.Mint)
	instructions = append(instructions, ataInstruction)

	return instructions, userATA, nil
}

// sendAndConfirmTransaction создает, подписывает, отправляет и ожидает подтверждения транзакции.
func (d *DEX) sendAndConfirmTransaction(ctx context.Context, instructions []solana.Instruction) (solana.Signature, error) {
	// 1) blockhash
	blockhash, err := d.client.GetRecentBlockhash(ctx)
	if err != nil {
		return solana.Signature{}, fmt.Errorf("get recent blockhash: %w", err)
	}

	// 2) сборка готовой транзакции
	tx, err := solana.NewTransaction(
		instructions,
		blockhash,
		solana.TransactionPayer(d.wallet.PublicKey),
		// сюда же при необходимости ALT:
		// solana.TransactionWithAddressLookupTables(d.addressTables...),
	)
	if err != nil {
		return solana.Signature{}, fmt.Errorf("create transaction: %w", err)
	}

	// 3) подпись
	if err := d.wallet.SignTransaction(tx); err != nil {
		return solana.Signature{}, fmt.Errorf("sign transaction: %w", err)
	}

	// 4) отправка с опциями для ускорения обработки
	txOpts := blockchain.TransactionOptions{
		SkipPreflight:       true,
		PreflightCommitment: rpc.CommitmentProcessed,
	}
	sig, err := d.client.SendTransactionWithOpts(ctx, tx, txOpts)
	if err != nil {
		return solana.Signature{}, fmt.Errorf("send transaction: %w", err)
	}
	d.logger.Info("Transaction sent", zap.String("signature", sig.String()))

	// 5) ожидание подтверждения (используем CommitmentProcessed для быстрого подтверждения)
	if err := d.client.WaitForTransactionConfirmation(ctx, sig, rpc.CommitmentProcessed); err != nil {
		d.logger.Warn("Confirm failed", zap.String("signature", sig.String()), zap.Error(err))
		return sig, fmt.Errorf("confirmation failed: %w", err)
	}
	d.logger.Info("Transaction confirmed", zap.String("signature", sig.String()))

	return sig, nil
}
