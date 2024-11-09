// internal/dex/types.go
package dex

import (
	"context"
	"fmt"

	"github.com/gagliardetto/solana-go"
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
	// Создание параметров свапа
	d.logger.Debug("Starting swap execution",
		zap.String("source_token", task.SourceToken),
		zap.String("target_token", task.TargetToken),
		zap.Float64("amount_in", task.AmountIn))

	// Получаем информацию о пуле
	pool, err := d.client.GetPool(ctx,
		solana.MustPublicKeyFromBase58(task.SourceToken),
		solana.MustPublicKeyFromBase58(task.TargetToken))
	if err != nil {
		return fmt.Errorf("failed to get pool info: %w", err)
	}

	// Конвертируем AmountIn в uint64
	amountInLamports := uint64(task.AmountIn * float64(solana.LAMPORTS_PER_SOL))

	// Создаем параметры свапа
	params := &raydium.SwapParams{
		UserWallet:          w.PublicKey,
		PrivateKey:          &w.PrivateKey,
		AmountIn:            amountInLamports,
		Pool:                pool,
		PriorityFeeLamports: uint64(task.PriorityFee * float64(solana.LAMPORTS_PER_SOL)),
	}

	// Выполняем свап
	signature, err := d.client.ExecuteSwap(params)
	if err != nil {
		return fmt.Errorf("swap failed: %w", err)
	}

	d.logger.Info("Swap executed successfully",
		zap.String("signature", signature),
		zap.String("pool", pool.ID.String()),
		zap.String("wallet", w.PublicKey.String()))

	return nil
}
