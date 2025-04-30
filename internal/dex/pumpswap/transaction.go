// =============================
// File: internal/dex/pumpswap/transaction.go
// =============================
package pumpswap

import (
	"context"
	"fmt"
	"github.com/cenkalti/backoff/v5"
	"github.com/gagliardetto/solana-go"
	computebudget "github.com/gagliardetto/solana-go/programs/compute-budget"
	"github.com/gagliardetto/solana-go/rpc"
	"strconv"
	"strings"
	"time"
)

// buildAndSubmitTransaction строит, подписывает и отправляет транзакцию.
//
// Метод объединяет процессы создания, подписи и отправки транзакции
// с механизмом повторных попыток. Он использует экспоненциальную стратегию
// задержки между попытками и имеет ограничение на общее время выполнения в 15 секунд.
func (d *DEX) buildAndSubmitTransaction(ctx context.Context, instructions []solana.Instruction) (solana.Signature, error) {
	op := func() (solana.Signature, error) {
		tx, err := d.createSignedTransaction(ctx, instructions)
		if err != nil {
			return solana.Signature{}, err
		}

		return d.submitAndConfirmTransaction(ctx, tx)
	}

	return backoff.Retry(
		ctx,
		op,
		backoff.WithBackOff(backoff.NewExponentialBackOff()),
		backoff.WithMaxElapsedTime(15*time.Second),
	)
}

// createSignedTransaction создает и подписывает новую транзакцию с указанными инструкциями.
//
// Метод получает актуальный blockhash, создает транзакцию с переданными инструкциями
// и подписывает её кошельком DEX. В случае критических ошибок (отсутствие blockhash,
// невозможность создать или подписать транзакцию) возвращается постоянная ошибка,
// которая предотвращает повторные попытки.
func (d *DEX) createSignedTransaction(ctx context.Context, instructions []solana.Instruction) (*solana.Transaction, error) {
	blockhash, err := d.client.GetRecentBlockhash(ctx)
	if err != nil {
		return nil, backoff.Permanent(fmt.Errorf("failed to get recent blockhash: %w", err))
	}

	tx, err := solana.NewTransaction(instructions, blockhash, solana.TransactionPayer(d.wallet.PublicKey))
	if err != nil {
		return nil, backoff.Permanent(fmt.Errorf("failed to create transaction: %w", err))
	}

	if err := d.wallet.SignTransaction(tx); err != nil {
		return nil, backoff.Permanent(fmt.Errorf("failed to sign transaction: %w", err))
	}

	return tx, nil
}

// submitAndConfirmTransaction отправляет транзакцию и ожидает ее подтверждения.
//
// Метод отправляет подписанную транзакцию в сеть Solana и ожидает ее подтверждения.
// Он обрабатывает различные типы ошибок: временные (BlockhashNotFound), специфические
// (SlippageExceeded) и постоянные. Для временных ошибок возможен повторный запуск,
// для постоянных - операция прерывается.
func (d *DEX) submitAndConfirmTransaction(ctx context.Context, tx *solana.Transaction) (solana.Signature, error) {
	sig, err := d.client.SendTransaction(ctx, tx)
	if err != nil {
		if strings.Contains(err.Error(), "BlockhashNotFound") {
			return solana.Signature{}, err // Временная ошибка для retry
		}

		// Проверка на известные ошибки
		if IsSlippageExceededError(err) {
			return solana.Signature{}, &SlippageExceededError{
				OriginalError: err,
			}
		}

		// Постоянная ошибка
		return solana.Signature{}, backoff.Permanent(fmt.Errorf("transaction failed: %w", err))
	}

	err = d.client.WaitForTransactionConfirmation(ctx, sig, rpc.CommitmentConfirmed)
	if err != nil {
		return sig, fmt.Errorf("transaction confirmed but with error: %w", err)
	}

	return sig, nil
}

// preparePriorityInstructions подготавливает инструкции для установки лимита и цены вычислительных единиц.
//
// Метод создает инструкции для управления вычислительными ресурсами транзакции:
// установка лимита вычислительных единиц и их стоимости (приоритетная комиссия).
// Приоритетная комиссия преобразуется из SOL в лампорты (1 SOL = 1e9 лампортов).
func (d *DEX) preparePriorityInstructions(computeUnits uint32, priorityFeeSol string) ([]solana.Instruction, error) {
	var instructions []solana.Instruction
	if computeUnits > 0 {
		instructions = append(instructions,
			computebudget.NewSetComputeUnitLimitInstruction(computeUnits).Build())
	}
	if priorityFeeSol != "" {
		fee, err := strconv.ParseFloat(priorityFeeSol, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid priority fee: %w", err)
		}
		// Перевод SOL в лампорты (1 SOL = 1e9 лампортов)
		feeLamports := uint64(fee * 1e9)
		if feeLamports > 0 {
			instructions = append(instructions,
				computebudget.NewSetComputeUnitPriceInstruction(feeLamports).Build())
		}
	}
	return instructions, nil
}

// prepareSwapParams создает структуру параметров для инструкции свапа.
//
// Метод собирает все необходимые параметры (адреса токенов, аккаунтов, программ)
// в единую структуру, которая используется для создания инструкции свопа.
// Параметры включают информацию о пуле, токен-аккаунтах пользователя,
// суммах и получателе комиссии протокола.
func (d *DEX) prepareSwapParams(
	pool *PoolInfo,
	accounts *PreparedTokenAccounts,
	isBuy bool,
	baseAmount uint64,
	quoteAmount uint64,
) *SwapInstructionParams {
	return &SwapInstructionParams{
		IsBuy:                            isBuy,
		PoolAddress:                      pool.Address,
		User:                             d.wallet.PublicKey,
		GlobalConfig:                     d.config.GlobalConfig,
		BaseMint:                         pool.BaseMint,
		QuoteMint:                        pool.QuoteMint,
		UserBaseTokenAccount:             accounts.UserBaseATA,
		UserQuoteTokenAccount:            accounts.UserQuoteATA,
		PoolBaseTokenAccount:             pool.PoolBaseTokenAccount,
		PoolQuoteTokenAccount:            pool.PoolQuoteTokenAccount,
		ProtocolFeeRecipient:             accounts.ProtocolFeeRecipient,
		ProtocolFeeRecipientTokenAccount: accounts.ProtocolFeeRecipientATA,
		BaseTokenProgram:                 TokenProgramID,
		QuoteTokenProgram:                TokenProgramID,
		EventAuthority:                   d.config.EventAuthority,
		ProgramID:                        d.config.ProgramID,
		Amount1:                          baseAmount,
		Amount2:                          quoteAmount,
	}
}

// buildSwapTransaction создает полный список инструкций для транзакции свопа.
//
// Метод формирует полный набор инструкций для выполнения свопа токенов.
// Инструкции выполняются в следующем порядке:
// 1) Приоритетные инструкции (установка лимита и цены CU)
// 2) Создание ассоциированных токен-аккаунтов пользователя (если не существуют)
// 3) Непосредственно инструкция свопа
func (d *DEX) buildSwapTransaction(
	pool *PoolInfo,
	accounts *PreparedTokenAccounts,
	isBuy bool,
	baseAmount, quoteAmount uint64,
	priorityInstructions []solana.Instruction,
) []solana.Instruction {
	var instructions []solana.Instruction
	instructions = append(instructions, priorityInstructions...)
	instructions = append(instructions, accounts.CreateBaseATAIx, accounts.CreateQuoteATAIx)

	swapParams := d.prepareSwapParams(pool, accounts, isBuy, baseAmount, quoteAmount)
	swapIx := createSwapInstruction(swapParams)
	instructions = append(instructions, swapIx)

	return instructions
}

// prepareTokenAccounts подготавливает ATA пользователя и инструкции для их создания.
//
// Метод вычисляет адреса ассоциированных токен-аккаунтов (ATA) для базового и
// квотного токенов, создает инструкции для их создания (в случае отсутствия)
// и получает информацию о получателе комиссии протокола из глобальной конфигурации.
func (d *DEX) prepareTokenAccounts(ctx context.Context, pool *PoolInfo) (*PreparedTokenAccounts, error) {
	userBaseATA, _, err := solana.FindAssociatedTokenAddress(d.wallet.PublicKey, pool.BaseMint)
	if err != nil {
		return nil, err
	}

	userQuoteATA, _, err := solana.FindAssociatedTokenAddress(d.wallet.PublicKey, pool.QuoteMint)
	if err != nil {
		return nil, err
	}

	createBaseATAIx := d.wallet.CreateAssociatedTokenAccountIdempotentInstruction(
		d.wallet.PublicKey, d.wallet.PublicKey, pool.BaseMint)
	createQuoteATAIx := d.wallet.CreateAssociatedTokenAccountIdempotentInstruction(
		d.wallet.PublicKey, d.wallet.PublicKey, pool.QuoteMint)

	globalConfig, err := d.getGlobalConfig(ctx)
	if err != nil {
		return nil, err
	}

	// Initialize with zero key and check if first recipient is non-zero
	protocolFeeRecipient := solana.PublicKeyFromBytes(make([]byte, 32))
	if !globalConfig.ProtocolFeeRecipients[0].IsZero() {
		protocolFeeRecipient = globalConfig.ProtocolFeeRecipients[0]
	}

	protocolFeeRecipientATA, _, err := solana.FindAssociatedTokenAddress(
		protocolFeeRecipient,
		pool.QuoteMint,
	)
	if err != nil {
		return nil, err
	}

	return &PreparedTokenAccounts{
		UserBaseATA:             userBaseATA,
		UserQuoteATA:            userQuoteATA,
		ProtocolFeeRecipientATA: protocolFeeRecipientATA,
		ProtocolFeeRecipient:    protocolFeeRecipient,
		CreateBaseATAIx:         createBaseATAIx,
		CreateQuoteATAIx:        createQuoteATAIx,
	}, nil
}
