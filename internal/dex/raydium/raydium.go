// internal/dex/raydium/raydium.go

package raydium

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/gagliardetto/solana-go"
	associatedtokenaccount "github.com/gagliardetto/solana-go/programs/associated-token-account"
	computebudget "github.com/gagliardetto/solana-go/programs/compute-budget"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain"
	"github.com/rovshanmuradov/solana-bot/internal/types"
	"github.com/rovshanmuradov/solana-bot/internal/wallet"
	"go.uber.org/zap"
)

const (
	defaultTimeout  = 10 * time.Second
	maxRetries      = 3
	retryDelay      = 500 * time.Millisecond
	ataCheckTimeout = 5 * time.Second
	txSendTimeout   = 15 * time.Second
)

func NewDEX(client blockchain.Client, logger *zap.Logger, poolInfo *Pool) *DEX {
	if err := validateDEXParams(client, logger, poolInfo); err != nil {
		logger.Error("Failed to create DEX", zap.Error(err))
		return nil
	}

	return &DEX{
		client:   client,
		logger:   logger.Named("raydium-dex"),
		poolInfo: poolInfo,
	}
}

func (r *DEX) ExecuteSwap(ctx context.Context, task *types.Task, userWallet *wallet.Wallet) error {
	opCtx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	logger := r.logger.With(
		zap.String("task", task.TaskName),
		zap.String("wallet", userWallet.PublicKey.String()),
	)
	logger.Info("Starting swap execution")

	// Проверяем и получаем токен-аккаунты с таймаутом
	sourceMint, targetMint, err := parseTokenAddresses(task.SourceToken, task.TargetToken)
	if err != nil {
		return fmt.Errorf("invalid token addresses: %w", err)
	}

	ataCtx, ataCancel := context.WithTimeout(opCtx, ataCheckTimeout)
	defer ataCancel()

	sourceATA, targetATA, err := r.setupTokenAccounts(ataCtx, userWallet, sourceMint, targetMint, logger)
	if err != nil {
		return fmt.Errorf("failed to setup token accounts: %w", err)
	}

	// Подготавливаем amount с учетом decimals
	amountIn := uint64(task.AmountIn * math.Pow10(task.SourceTokenDecimals))
	minAmountOut := uint64(task.MinAmountOut * math.Pow10(task.TargetTokenDecimals))

	logger.Debug("Prepared swap amounts",
		zap.Uint64("amount_in", amountIn),
		zap.Uint64("min_amount_out", minAmountOut))

	// Создаем инструкции с таймаутом
	swapCtx, swapCancel := context.WithTimeout(opCtx, txSendTimeout)
	defer swapCancel()

	// Подготавливаем все необходимые инструкции
	instructions, err := r.PrepareSwapInstructions(
		swapCtx,
		userWallet.PublicKey,
		sourceATA,
		targetATA,
		amountIn,
		minAmountOut,
		task.PriorityFee,
		logger,
	)
	if err != nil {
		return fmt.Errorf("failed to prepare swap instructions: %w", err)
	}

	// Отправляем транзакцию
	signature, err := r.sendTransactionWithRetry(swapCtx, userWallet, instructions, logger)
	if err != nil {
		return fmt.Errorf("failed to send swap transaction: %w", err)
	}

	logger.Info("Swap transaction sent successfully",
		zap.String("signature", signature.String()),
		zap.Float64("priority_fee", task.PriorityFee))

	return nil
}

func (r *DEX) setupTokenAccounts(
	ctx context.Context,
	wallet *wallet.Wallet,
	sourceMint, targetMint solana.PublicKey,
	logger *zap.Logger,
) (solana.PublicKey, solana.PublicKey, error) {
	sourceATA, _, err := solana.FindAssociatedTokenAddress(wallet.PublicKey, sourceMint)
	if err != nil {
		return solana.PublicKey{}, solana.PublicKey{}, fmt.Errorf("failed to find source ATA: %w", err)
	}

	targetATA, _, err := solana.FindAssociatedTokenAddress(wallet.PublicKey, targetMint)
	if err != nil {
		return solana.PublicKey{}, solana.PublicKey{}, fmt.Errorf("failed to find target ATA: %w", err)
	}

	// Проверяем и создаем ATA если необходимо
	if err := r.ensureATA(ctx, wallet, sourceMint, sourceATA, "source", logger); err != nil {
		return solana.PublicKey{}, solana.PublicKey{}, err
	}

	if err := r.ensureATA(ctx, wallet, targetMint, targetATA, "target", logger); err != nil {
		return solana.PublicKey{}, solana.PublicKey{}, err
	}

	return sourceATA, targetATA, nil
}

func (r *DEX) ensureATA(
	ctx context.Context,
	wallet *wallet.Wallet,
	mint, ata solana.PublicKey,
	ataType string,
	logger *zap.Logger,
) error {
	account, err := r.client.GetAccountInfo(ctx, ata)
	if err != nil {
		return fmt.Errorf("failed to check %s ATA: %w", ataType, err)
	}

	if account.Value == nil {
		logger.Debug("Creating ATA", zap.String("type", ataType), zap.String("address", ata.String()))

		instruction := associatedtokenaccount.NewCreateInstruction(
			wallet.PublicKey,
			wallet.PublicKey,
			mint,
		).Build()

		if err := r.sendATATransaction(ctx, wallet, instruction); err != nil {
			return fmt.Errorf("failed to create %s ATA: %w", ataType, err)
		}

		logger.Debug("ATA created successfully", zap.String("type", ataType))
	}

	return nil
}

// Добавляем метод sendATATransaction
func (r *DEX) sendATATransaction(ctx context.Context, wallet *wallet.Wallet, instruction solana.Instruction) error {
	logger := r.logger.With(
		zap.String("wallet", wallet.PublicKey.String()),
		zap.String("operation", "create_ata"),
	)

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			recent, err := r.client.GetRecentBlockhash(ctx)
			if err != nil {
				lastErr = fmt.Errorf("failed to get recent blockhash: %w", err)
				continue
			}

			tx, err := solana.NewTransaction(
				[]solana.Instruction{instruction},
				recent,
				solana.TransactionPayer(wallet.PublicKey),
			)
			if err != nil {
				lastErr = fmt.Errorf("failed to create ATA transaction: %w", err)
				continue
			}

			_, err = tx.Sign(func(key solana.PublicKey) *solana.PrivateKey {
				if key.Equals(wallet.PublicKey) {
					return &wallet.PrivateKey
				}
				return nil
			})
			if err != nil {
				lastErr = fmt.Errorf("failed to sign ATA transaction: %w", err)
				continue
			}

			sig, err := r.client.SendTransaction(ctx, tx)
			if err != nil {
				lastErr = err
				logger.Warn("Failed to send ATA transaction, retrying",
					zap.Int("attempt", attempt+1),
					zap.Error(err))
				time.Sleep(retryDelay)
				continue
			}

			logger.Debug("ATA transaction sent successfully",
				zap.String("signature", sig.String()))
			return nil
		}
	}

	return fmt.Errorf("failed to send ATA transaction after %d attempts: %w", maxRetries, lastErr)
}

// Добавляем метод PrepareSwapInstruction для соответствия интерфейсу types.DEX
// PrepareSwapInstructions объединяет все инструкции для свапа
func (r *DEX) PrepareSwapInstructions(
	ctx context.Context,
	wallet solana.PublicKey,
	sourceATA solana.PublicKey,
	targetATA solana.PublicKey,
	amountIn uint64,
	minAmountOut uint64,
	priorityFee float64,
	logger *zap.Logger,
) ([]solana.Instruction, error) {
	var instructions []solana.Instruction

	// Добавляем compute budget инструкции
	computeBudgetInst := computebudget.NewSetComputeUnitPriceInstruction(
		uint64(priorityFee * 1e6),
	).Build()
	instructions = append(instructions, computeBudgetInst)

	// Создаем базовую инструкцию свапа
	swapInst, err := r.PrepareSwapInstruction(
		ctx,
		wallet,
		sourceATA,
		targetATA,
		amountIn,
		minAmountOut,
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare swap instruction: %w", err)
	}
	instructions = append(instructions, swapInst)

	return instructions, nil
}

// PrepareSwapInstruction подготавливает базовую инструкцию свапа
func (r *DEX) PrepareSwapInstruction(
	ctx context.Context,
	wallet solana.PublicKey,
	sourceATA solana.PublicKey,
	targetATA solana.PublicKey,
	amountIn uint64,
	minAmountOut uint64,
	logger *zap.Logger,
) (solana.Instruction, error) {
	logger = logger.With(
		zap.String("wallet", wallet.String()),
		zap.String("source_ata", sourceATA.String()),
		zap.String("target_ata", targetATA.String()),
	)
	logger.Debug("Preparing swap instruction")

	// Создаем инструкцию свапа
	instruction, err := r.CreateSwapInstruction(
		wallet,
		sourceATA,
		targetATA,
		amountIn,
		minAmountOut,
		logger,
		r.poolInfo,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create swap instruction: %w", err)
	}

	logger.Debug("Swap instruction prepared successfully")
	return instruction, nil
}

func (r *DEX) sendTransactionWithRetry(
	ctx context.Context,
	wallet *wallet.Wallet,
	instructions []solana.Instruction,
	logger *zap.Logger,
) (solana.Signature, error) {
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return solana.Signature{}, ctx.Err()
		default:
			signature, err := r.sendTransaction(ctx, wallet, instructions)
			if err == nil {
				return signature, nil
			}
			lastErr = err
			logger.Warn("Retrying transaction send",
				zap.Int("attempt", attempt+1),
				zap.Error(err))
			time.Sleep(retryDelay)
		}
	}
	return solana.Signature{}, fmt.Errorf("failed after %d attempts: %w", maxRetries, lastErr)
}

func (r *DEX) sendTransaction(
	ctx context.Context,
	wallet *wallet.Wallet,
	instructions []solana.Instruction,
) (solana.Signature, error) {
	recent, err := r.client.GetRecentBlockhash(ctx)
	if err != nil {
		return solana.Signature{}, fmt.Errorf("failed to get recent blockhash: %w", err)
	}

	tx, err := solana.NewTransaction(
		instructions,
		recent,
		solana.TransactionPayer(wallet.PublicKey),
	)
	if err != nil {
		return solana.Signature{}, fmt.Errorf("failed to create transaction: %w", err)
	}

	_, err = tx.Sign(func(key solana.PublicKey) *solana.PrivateKey {
		if key.Equals(wallet.PublicKey) {
			return &wallet.PrivateKey
		}
		return nil
	})
	if err != nil {
		return solana.Signature{}, fmt.Errorf("failed to sign transaction: %w", err)
	}

	return r.client.SendTransaction(ctx, tx)
}

func validateDEXParams(client blockchain.Client, logger *zap.Logger, poolInfo *Pool) error {
	switch {
	case client == nil:
		return fmt.Errorf("client cannot be nil")
	case logger == nil:
		return fmt.Errorf("logger cannot be nil")
	case poolInfo == nil:
		return fmt.Errorf("pool info cannot be nil")
	}
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
