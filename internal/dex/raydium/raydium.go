// internal/dex/raydium/raydium.go
package raydium

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/gagliardetto/solana-go"
	associatedtokenaccount "github.com/gagliardetto/solana-go/programs/associated-token-account"
	computebudget "github.com/gagliardetto/solana-go/programs/compute-budget"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain"
	"github.com/rovshanmuradov/solana-bot/internal/types"
	"github.com/rovshanmuradov/solana-bot/internal/wallet"
	"go.uber.org/zap"
)

// SwapParams содержит параметры для выполнения свапа
type SwapParams struct {
	SourceMint  solana.PublicKey
	TargetMint  solana.PublicKey
	Amount      float64
	MinAmount   float64
	Decimals    int
	PriorityFee float64
}

// NewDEX создает новый экземпляр Raydium DEX
func NewDEX(client blockchain.Client, logger *zap.Logger, poolInfo *Pool) *DEX {
	// Базовая валидация с информативными ошибками
	switch {
	case client == nil:
		if logger != nil {
			logger.Error("Failed to create DEX: client is nil")
		}
		return nil
	case logger == nil:
		// Если логгер nil, используем только возврат, так как логировать некуда
		return nil
	case poolInfo == nil:
		logger.Error("Failed to create DEX: pool info is nil")
		return nil
	}

	// Дополнительная валидация конфигурации пула
	if err := ValidatePool(poolInfo); err != nil {
		logger.Error("Failed to create DEX: invalid pool configuration",
			zap.Error(err),
			zap.String("amm_id", poolInfo.AmmID),
			zap.String("program_id", poolInfo.AmmProgramID))
		return nil
	}

	dex := &DEX{
		client:   client,
		logger:   logger.Named("raydium-dex"), // Добавляем префикс для логов
		poolInfo: poolInfo,
	}

	// Логируем успешное создание только в debug режиме
	logger.Debug("Raydium DEX instance created successfully",
		zap.String("amm_id", poolInfo.AmmID),
		zap.String("program_id", poolInfo.AmmProgramID))

	return dex
}

func (r *DEX) PrepareSwapInstruction(
	ctx context.Context,
	userWallet solana.PublicKey,
	userSourceTokenAccount solana.PublicKey,
	userDestinationTokenAccount solana.PublicKey,
	amountIn uint64,
	minAmountOut uint64,
	logger *zap.Logger,
) (solana.Instruction, error) {
	// Быстрая проверка контекста
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Проверяем DEX и pool info перед другими операциями
	if r == nil || r.poolInfo == nil {
		return nil, fmt.Errorf("invalid DEX configuration")
	}

	// Валидация входных данных одним блоком для производительности
	var invalidInputs []string
	switch {
	case userWallet.IsZero():
		invalidInputs = append(invalidInputs, "wallet")
	case userSourceTokenAccount.IsZero():
		invalidInputs = append(invalidInputs, "source account")
	case userDestinationTokenAccount.IsZero():
		invalidInputs = append(invalidInputs, "destination account")
	case amountIn == 0:
		invalidInputs = append(invalidInputs, "amount in")
	}

	if len(invalidInputs) > 0 {
		err := fmt.Errorf("invalid input parameters: %s", strings.Join(invalidInputs, ", "))
		logger.Error("Swap instruction preparation failed",
			zap.Error(err),
			zap.Strings("invalid_inputs", invalidInputs))
		return nil, err
	}

	// Используем defer для логирования только в случае ошибки
	var instruction solana.Instruction
	var err error
	defer func() {
		if err != nil {
			logger.Error("Failed to prepare swap instruction",
				zap.Error(err),
				zap.String("wallet", userWallet.String()),
				zap.Uint64("amount_in", amountIn),
				zap.Uint64("min_amount_out", minAmountOut))
		}
	}()

	// Создаем инструкцию напрямую, без использования горутины
	instruction, err = r.CreateSwapInstruction(
		userWallet,
		userSourceTokenAccount,
		userDestinationTokenAccount,
		amountIn,
		minAmountOut,
		logger,
		r.poolInfo,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create swap instruction: %w", err)
	}

	// Логируем успешное создание только в debug режиме
	if logger.Core().Enabled(zap.DebugLevel) {
		logger.Debug("Swap instruction prepared successfully",
			zap.String("wallet", userWallet.String()),
			zap.Uint64("amount_in", amountIn),
			zap.Uint64("min_amount_out", minAmountOut))
	}

	return instruction, nil
}

func (r *DEX) ExecuteSwap(
	ctx context.Context,
	task *types.Task,
	userWallet *wallet.Wallet,
) error {
	start := time.Now()
	logger := r.logger.With(
		zap.String("wallet", userWallet.PublicKey.String()),
		zap.String("task", task.TaskName),
	)

	// Базовая валидация
	if task.AmountIn <= 0 || task.MinAmountOut <= 0 {
		return fmt.Errorf("invalid amounts: in=%v, min_out=%v", task.AmountIn, task.MinAmountOut)
	}

	// Конвертация адресов токенов
	sourceMint, targetMint, err := parseTokenAddresses(task.SourceToken, task.TargetToken)
	if err != nil {
		return fmt.Errorf("failed to parse token addresses: %w", err)
	}

	// Получение токен-аккаунтов, передаем весь wallet
	sourceATA, targetATA, err := r.getOrCreateTokenAccounts(
		ctx,
		userWallet, // передаем весь wallet, а не только PublicKey
		sourceMint,
		targetMint,
	)
	if err != nil {
		return fmt.Errorf("failed to get token accounts: %w", err)
	}

	// Подготовка amount с учетом decimals
	amountIn := uint64(task.AmountIn * math.Pow10(task.SourceTokenDecimals))
	minAmountOut := uint64(task.MinAmountOut * math.Pow10(task.TargetTokenDecimals))

	// Получение инструкции свапа
	swapInstruction, err := r.PrepareSwapInstruction(
		ctx,
		userWallet.PublicKey,
		sourceATA,
		targetATA,
		amountIn,
		minAmountOut,
		logger,
	)
	if err != nil {
		return fmt.Errorf("failed to prepare swap instruction: %w", err)
	}

	// Выполнение транзакции
	signature, err := r.executeTransaction(ctx, userWallet, swapInstruction, task.PriorityFee)
	if err != nil {
		logger.Error("Swap failed",
			zap.Error(err),
			zap.Duration("duration", time.Since(start)),
		)
		return fmt.Errorf("swap failed: %w", err)
	}

	logger.Info("Swap completed",
		zap.String("signature", signature.String()),
		zap.Duration("duration", time.Since(start)),
	)

	return nil
}

func parseTokenAddresses(sourceToken, targetToken string) (solana.PublicKey, solana.PublicKey, error) {
	sourceMint, err := solana.PublicKeyFromBase58(sourceToken)
	if err != nil {
		return solana.PublicKey{}, solana.PublicKey{}, fmt.Errorf("invalid source token: %w", err)
	}

	targetMint, err := solana.PublicKeyFromBase58(targetToken)
	if err != nil {
		return solana.PublicKey{}, solana.PublicKey{}, fmt.Errorf("invalid target token: %w", err)
	}

	return sourceMint, targetMint, nil
}

func (r *DEX) getOrCreateTokenAccounts(
	ctx context.Context,
	wallet *wallet.Wallet,
	sourceMint, targetMint solana.PublicKey,
) (solana.PublicKey, solana.PublicKey, error) {
	// Находим адреса ATA, используя PublicKey из wallet
	sourceATA, _, err := solana.FindAssociatedTokenAddress(wallet.PublicKey, sourceMint)
	if err != nil {
		return solana.PublicKey{}, solana.PublicKey{}, fmt.Errorf("source ATA error: %w", err)
	}

	targetATA, _, err := solana.FindAssociatedTokenAddress(wallet.PublicKey, targetMint)
	if err != nil {
		return solana.PublicKey{}, solana.PublicKey{}, fmt.Errorf("target ATA error: %w", err)
	}

	r.logger.Debug("Checking source ATA",
		zap.String("source_ata", sourceATA.String()),
		zap.String("wallet", wallet.PublicKey.String()),
		zap.String("source_mint", sourceMint.String()))

	r.logger.Debug("Checking target ATA",
		zap.String("target_ata", targetATA.String()),
		zap.String("wallet", wallet.PublicKey.String()),
		zap.String("target_mint", targetMint.String()))

	// Проверяем существование source ATA с таймаутом
	ctxWithTimeout, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	sourceAccount, err := r.client.GetAccountInfo(ctxWithTimeout, sourceATA)
	if err != nil {
		return solana.PublicKey{}, solana.PublicKey{}, fmt.Errorf("failed to check source ATA: %w", err)
	}

	// Если source ATA не существует, создаем его
	if sourceAccount.Value == nil {
		r.logger.Debug("Creating source ATA",
			zap.String("ata", sourceATA.String()),
			zap.String("mint", sourceMint.String()))

		createATAInstr := associatedtokenaccount.NewCreateInstruction(
			wallet.PublicKey, // payer
			wallet.PublicKey, // owner
			sourceMint,       // mint
		).Build()

		if err := r.sendATATransaction(ctx, wallet, createATAInstr); err != nil {
			return solana.PublicKey{}, solana.PublicKey{}, fmt.Errorf("failed to create source ATA: %w", err)
		}
	}

	// Проверяем существование target ATA с таймаутом
	ctxWithTimeout2, cancel2 := context.WithTimeout(ctx, 10*time.Second)
	defer cancel2()

	targetAccount, err := r.client.GetAccountInfo(ctxWithTimeout2, targetATA)
	if err != nil {
		return solana.PublicKey{}, solana.PublicKey{}, fmt.Errorf("failed to check target ATA: %w", err)
	}

	// Если target ATA не существует, создаем его
	if targetAccount.Value == nil {
		r.logger.Debug("Creating target ATA",
			zap.String("ata", targetATA.String()),
			zap.String("mint", targetMint.String()))

		createATAInstr := associatedtokenaccount.NewCreateInstruction(
			wallet.PublicKey, // payer
			wallet.PublicKey, // owner
			targetMint,       // mint
		).Build()

		if err := r.sendATATransaction(ctx, wallet, createATAInstr); err != nil {
			return solana.PublicKey{}, solana.PublicKey{}, fmt.Errorf("failed to create target ATA: %w", err)
		}
	}

	return sourceATA, targetATA, nil
}

// Вспомогательный метод для отправки транзакции создания ATA
func (r *DEX) sendATATransaction(ctx context.Context, wallet *wallet.Wallet, instruction solana.Instruction) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		recent, err := r.client.GetRecentBlockhash(ctx)
		if err != nil {
			return fmt.Errorf("failed to get recent blockhash: %w", err)
		}

		tx, err := solana.NewTransaction(
			[]solana.Instruction{instruction},
			recent,
			solana.TransactionPayer(wallet.PublicKey),
		)
		if err != nil {
			return fmt.Errorf("failed to create transaction: %w", err)
		}

		// Подписываем транзакцию
		_, err = tx.Sign(func(key solana.PublicKey) *solana.PrivateKey {
			if key.Equals(wallet.PublicKey) {
				return &wallet.PrivateKey
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("failed to sign transaction: %w", err)
		}

		// Отправляем транзакцию
		sig, err := r.client.SendTransaction(ctx, tx)
		if err != nil {
			return fmt.Errorf("failed to send transaction: %w", err)
		}

		// Проверяем статус через GetAccountInfo
		for i := 0; i < 50; i++ { // максимум 25 секунд ожидания
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				time.Sleep(500 * time.Millisecond)

				resp, err := r.client.GetAccountInfo(ctx, wallet.PublicKey)
				if err != nil {
					continue
				}

				if resp != nil && resp.Value != nil {
					return nil // аккаунт создан
				}
			}
		}

		return fmt.Errorf("timeout waiting for transaction %s confirmation", sig)
	}
}

func (r *DEX) executeTransaction(
	ctx context.Context,
	wallet *wallet.Wallet,
	instruction solana.Instruction,
	priorityFee float64,
) (solana.Signature, error) {
	// Получение последнего blockhash
	blockhash, err := r.client.GetRecentBlockhash(ctx)
	if err != nil {
		return solana.Signature{}, fmt.Errorf("failed to get blockhash: %w", err)
	}

	// Создание транзакции с приоритетом
	tx, err := solana.NewTransaction(
		[]solana.Instruction{
			ComputeBudgetInstruction(priorityFee),
			instruction,
		},
		blockhash,
		solana.TransactionPayer(wallet.PublicKey),
	)
	if err != nil {
		return solana.Signature{}, fmt.Errorf("failed to create transaction: %w", err)
	}

	// Подпись транзакции
	_, err = tx.Sign(func(key solana.PublicKey) *solana.PrivateKey {
		if key.Equals(wallet.PublicKey) {
			return &wallet.PrivateKey
		}
		return nil
	})
	if err != nil {
		return solana.Signature{}, fmt.Errorf("failed to sign transaction: %w", err)
	}

	// Отправка транзакции
	sig, err := r.client.SendTransaction(ctx, tx)
	if err != nil {
		return solana.Signature{}, fmt.Errorf("failed to send transaction: %w", err)
	}

	return sig, nil
}

func ComputeBudgetInstruction(priorityFee float64) solana.Instruction {
	return computebudget.NewSetComputeUnitPriceInstruction(
		uint64(priorityFee * 1e6), // Конвертация в микро-лампорты
	).Build()
}
