// =============================
// File: internal/dex/pumpfun/transactions.go
// =============================
// Package pumpfun содержит имплементацию для взаимодействия с протоколом Pump.fun на блокчейне Solana.
// Данный файл реализует функциональность для создания, подписи и отправки транзакций.
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
//
// Метод оборачивает исходный контекст в контекст с таймаутом, что позволяет
// ограничить максимальное время выполнения операции. Это важно для обеспечения
// отзывчивости системы и предотвращения бесконечного ожидания при проблемах с сетью.
//
// Параметры:
//   - ctx: исходный контекст, который может содержать значения и отмену
//   - timeout: максимальная длительность операции
//
// Возвращает:
//   - context.Context: новый контекст с таймаутом
//   - context.CancelFunc: функцию для отмены контекста до истечения таймаута
func (d *DEX) prepareTransactionContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, timeout)
}

// prepareBaseInstructions подготавливает базовые инструкции для транзакции.
//
// Метод создает набор инструкций, которые необходимы для большинства транзакций
// на Pump.fun: задание приоритета и комиссии для транзакции, а также проверка
// и при необходимости создание ассоциированного токен-аккаунта (ATA) пользователя.
//
// Параметры:
//   - ctx: контекст выполнения (не используется в текущей реализации)
//   - priorityFeeSol: комиссия приоритета в SOL (строковое представление)
//   - computeUnits: количество вычислительных единиц для транзакции
//
// Возвращает:
//   - []solana.Instruction: массив базовых инструкций для транзакции
//   - solana.PublicKey: адрес ассоциированного токен-аккаунта пользователя
//   - error: ошибку, если не удалось создать инструкции
func (d *DEX) prepareBaseInstructions(_ context.Context, priorityFeeSol string, computeUnits uint32) ([]solana.Instruction, solana.PublicKey, error) {
	// Шаг 1: Создаем инструкции для установки приоритета транзакции
	// Эти инструкции позволяют указать, сколько SOL валидаторы получат за обработку транзакции
	// и лимит вычислительных единиц (computeUnits), которые транзакция может использовать
	priorityInstructions, err := d.priorityManager.CreatePriorityInstructions(priorityFeeSol, computeUnits)
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
//
// Метод выполняет полный цикл работы с транзакцией: от создания до получения подтверждения
// от сети Solana. Он также включает симуляцию транзакции перед отправкой для выявления
// потенциальных проблем и оценки используемых вычислительных ресурсов.
//
// Параметры:
//   - ctx: контекст выполнения с возможностью отмены или таймаута
//   - instructions: массив инструкций для включения в транзакцию
//
// Возвращает:
//   - solana.Signature: подпись транзакции, которая может использоваться для отслеживания
//   - error: ошибку, если транзакция не удалась на любом этапе
func (d *DEX) sendAndConfirmTransaction(ctx context.Context, instructions []solana.Instruction) (solana.Signature, error) {
	// Шаг 1: Получаем актуальный blockhash из сети
	// Blockhash нужен для защиты от повторной отправки транзакции и имеет ограниченный срок действия
	blockhash, err := d.client.GetRecentBlockhash(ctx)
	if err != nil {
		return solana.Signature{}, fmt.Errorf("failed to get recent blockhash: %w", err)
	}
	d.logger.Debug("Got blockhash", zap.String("blockhash", blockhash.String()))

	// Шаг 2: Создаем транзакцию с указанными инструкциями, blockhash и плательщиком комиссии
	tx, err := solana.NewTransaction(
		instructions,
		blockhash,
		solana.TransactionPayer(d.wallet.PublicKey),
	)
	if err != nil {
		return solana.Signature{}, fmt.Errorf("failed to create transaction: %w", err)
	}

	// Шаг 3: Подписываем транзакцию приватным ключом кошелька
	if err := d.wallet.SignTransaction(tx); err != nil {
		return solana.Signature{}, fmt.Errorf("failed to sign transaction: %w", err)
	}

	// Шаг 4: Симулируем транзакцию для проверки корректности и оценки ресурсов
	// Это не гарантирует успешное выполнение, но позволяет выявить очевидные проблемы
	simResult, err := d.client.SimulateTransaction(ctx, tx)
	if err != nil || (simResult != nil && simResult.Err != nil) {
		// Логируем предупреждение, но продолжаем выполнение
		// Иногда симуляция может не пройти, но реальная транзакция выполнится успешно
		d.logger.Warn("Transaction simulation failed",
			zap.Error(err),
			zap.Any("sim_error", simResult != nil && simResult.Err != nil))
		// Продолжаем выполнение, так как симуляция может иногда не проходить для валидных транзакций
	} else {
		d.logger.Info("Transaction simulation successful",
			zap.Uint64("compute_units", simResult.UnitsConsumed))
	}

	// Шаг 5: Отправляем транзакцию в сеть
	txSig, err := d.client.SendTransaction(ctx, tx)
	if err != nil {
		return solana.Signature{}, fmt.Errorf("failed to send transaction: %w", err)
	}

	// Логируем успешную отправку транзакции
	d.logger.Info("Transaction sent successfully",
		zap.String("signature", txSig.String()))

	// Шаг 6: Ждем подтверждения транзакции сетью
	// Уровень подтверждения "Confirmed" означает, что транзакция включена в блок
	// и подтверждена большинством валидаторов
	if err := d.client.WaitForTransactionConfirmation(ctx, txSig, rpc.CommitmentConfirmed); err != nil {
		// Транзакция отправлена, но не получила подтверждения в указанное время
		d.logger.Warn("Failed to confirm transaction",
			zap.String("signature", txSig.String()),
			zap.Error(err))
		return txSig, fmt.Errorf("transaction confirmation failed: %w", err)
	}

	// Логируем успешное подтверждение транзакции
	d.logger.Info("Transaction confirmed",
		zap.String("signature", txSig.String()))
	return txSig, nil
}
