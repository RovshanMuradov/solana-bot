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

// prepareTransactionContext создает контекст с таймаутом для операции.
func (d *DEX) prepareTransactionContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, timeout)
}

// prepareBaseInstructions подготавливает базовые инструкции для транзакции.
func (d *DEX) prepareBaseInstructions(_ context.Context, priorityFeeSol string, computeUnits uint32) ([]solana.Instruction, solana.PublicKey, error) {
	// Шаг 1: Создаем инструкции для установки приоритета транзакции
	// Эти инструкции позволяют указать, сколько SOL валидаторы получат за обработку транзакции
	// и лимит вычислительных единиц (computeUnits), которые транзакция может использовать
	priorityInstructions, err := d.priorityManager.CreatePriorityInstructions(priorityFeeSol, computeUnits) // TODO: чекнуть работу CreatePriorityInstructions
	if err != nil {
		return nil, solana.PublicKey{}, fmt.Errorf("failed to create priority instructions: %w", err)
	}

	// Шаг 2: Вычисляем адрес ассоциированного токен-аккаунта (ATA) пользователя
	// ATA - это детерминированный адрес, который вычисляется на основе адреса кошелька пользователя и минта токена
	userATA, _, err := solana.FindAssociatedTokenAddress(d.wallet.PublicKey, d.config.Mint)
	if err != nil {
		return nil, solana.PublicKey{}, fmt.Errorf("failed to derive associated token account: %w", err)
	}

	// Шаг 3: Создаем инструкцию для проверки существования ATA и создания его при необходимости
	// Idempotent-инструкция не выдаст ошибку, если аккаунт уже существует
	ataInstruction := d.wallet.CreateAssociatedTokenAccountIdempotentInstruction(
		d.wallet.PublicKey, d.wallet.PublicKey, d.config.Mint)

	// Шаг 4: Объединяем все инструкции в единый массив
	var instructions []solana.Instruction
	instructions = append(instructions, priorityInstructions...)
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

	// 4) симуляция (оставляем ваш код)
	simResult, simErr := d.client.SimulateTransaction(ctx, tx)
	if simErr != nil || (simResult != nil && simResult.Err != nil) {
		d.logger.Warn("Transaction simulation failed", zap.Error(simErr), zap.Any("sim_error", simResult != nil && simResult.Err != nil))
	} else {
		d.logger.Info("Transaction simulation successful", zap.Uint64("compute_units", simResult.UnitsConsumed))
	}

	// 5) отправка
	sig, err := d.client.SendTransaction(ctx, tx)
	if err != nil {
		return solana.Signature{}, fmt.Errorf("send transaction: %w", err)
	}
	d.logger.Info("Transaction sent", zap.String("signature", sig.String()))

	// 6) ожидание подтверждения
	if err := d.client.WaitForTransactionConfirmation(ctx, sig, rpc.CommitmentConfirmed); err != nil {
		d.logger.Warn("Confirm failed", zap.String("signature", sig.String()), zap.Error(err))
		return sig, fmt.Errorf("confirmation failed: %w", err)
	}
	d.logger.Info("Transaction confirmed", zap.String("signature", sig.String()))

	return sig, nil
}
