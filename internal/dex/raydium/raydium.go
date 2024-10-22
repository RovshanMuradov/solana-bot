// internal/dex/raydium/raydium.go
package raydium

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/gagliardetto/solana-go"
	solanaClient "github.com/rovshanmuradov/solana-bot/internal/blockchain/solana"
	"github.com/rovshanmuradov/solana-bot/internal/types"
	"github.com/rovshanmuradov/solana-bot/internal/wallet"
	"go.uber.org/zap"
)

func NewDEX(client *solanaClient.Client, logger *zap.Logger, poolInfo *Pool) *DEX {
	return &DEX{
		client:   client,
		logger:   logger,
		poolInfo: poolInfo,
	}
}

func (r *DEX) Name() string {
	return "Raydium"
}

func (r *DEX) PrepareSwapInstruction(
	ctx context.Context,
	wallet solana.PublicKey,
	sourceToken solana.PublicKey,
	destinationToken solana.PublicKey,
	amountIn uint64,
	minAmountOut uint64,
	logger *zap.Logger,
) (solana.Instruction, error) {
	// Создаем канал для получения результата
	type result struct {
		instruction solana.Instruction
		err         error
	}
	resCh := make(chan result, 1)

	// Запускаем подготовку инструкции в отдельной горутине
	go func() {
		instruction, err := r.CreateSwapInstruction(
			wallet,
			sourceToken,
			destinationToken,
			amountIn,
			minAmountOut,
			logger,
			r.poolInfo,
		)
		resCh <- result{instruction, err}
	}()

	// Ожидаем либо завершения операции, либо отмены контекста
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("preparation cancelled: %w", ctx.Err())
	case res := <-resCh:
		if res.err != nil {
			return nil, fmt.Errorf("failed to create swap instruction: %w", res.err)
		}
		return res.instruction, nil
	}
}

// ExecuteSwap выполняет свап токенов на Raydium
func (r *DEX) ExecuteSwap(
	ctx context.Context,
	task *types.Task,
	wallet *wallet.Wallet,
) error {
	// Создаем контекст с таймаутом для подготовки инструкции
	prepareCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Конвертируем float64 в uint64 с учетом десятичных знаков
	amountIn := uint64(task.AmountIn * math.Pow10(task.SourceTokenDecimals))
	minAmountOut := uint64(task.MinAmountOut * math.Pow10(task.TargetTokenDecimals))

	// Подготавливаем инструкцию свапа
	swapInstruction, err := r.PrepareSwapInstruction(
		prepareCtx,
		wallet.PublicKey,
		task.UserSourceTokenAccount,
		task.UserDestinationTokenAccount,
		amountIn,
		minAmountOut,
		r.logger,
	)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("swap instruction preparation timed out: %w", err)
		}
		return fmt.Errorf("failed to prepare swap instruction: %w", err)
	}

	// Используем подготовленную инструкцию в транзакции
	return r.PrepareAndSendTransaction(ctx, task, wallet, r.logger, swapInstruction)
}
