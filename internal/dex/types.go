// internal/dex/types.go
package dex

import (
	"context"
	"fmt"

	"github.com/rovshanmuradov/solana-bot/internal/blockchain"
	"github.com/rovshanmuradov/solana-bot/internal/dex/raydium"
	"github.com/rovshanmuradov/solana-bot/internal/types"
	"github.com/rovshanmuradov/solana-bot/internal/wallet"
	"go.uber.org/zap"
)

// raydiumDEX реализует интерфейс types.DEX
type raydiumDEX struct {
	client *raydium.Client
	logger *zap.Logger
	config *raydium.SniperConfig
}

func (d *raydiumDEX) GetName() string {
	return "Raydium"
}

func (d *raydiumDEX) GetClient() blockchain.Client {
	return d.client.GetBaseClient()
}

func (d *raydiumDEX) GetConfig() interface{} {
	return d.config
}

func (d *raydiumDEX) ExecuteSwap(ctx context.Context, task *types.Task, w *wallet.Wallet) error {
	d.logger.Debug("Starting swap execution",
		zap.String("source_token", task.SourceToken),
		zap.String("target_token", task.TargetToken),
		zap.Float64("amount_in", task.AmountIn))

	// 1. Валидация и подготовка параметров свапа
	swapParams, err := d.validateAndPrepareSwap(ctx, task, w)
	if err != nil {
		return fmt.Errorf("failed to validate and prepare swap: %w", err)
	}

	// 2. Выполняем свап с повторными попытками
	result, err := d.client.RetrySwap(ctx, swapParams)
	if err != nil {
		return fmt.Errorf("swap execution failed: %w", err)
	}

	// 3. Проверяем результат свапа
	if err := d.client.ValidateSwapResult(ctx, result, swapParams); err != nil {
		return fmt.Errorf("swap result validation failed: %w", err)
	}

	d.logger.Info("Swap executed successfully",
		zap.String("signature", result.Signature.String()),
		zap.Uint64("amount_in", result.AmountIn),
		zap.Uint64("amount_out", result.AmountOut),
		zap.Duration("execution_time", result.ExecutionTime))

	return nil
}
