// internal/dex/raydium/raydium.go
package raydium

import (
	"context"

	"github.com/gagliardetto/solana-go"
	solanaClient "github.com/rovshanmuradov/solana-bot/internal/blockchain/solana"
	"github.com/rovshanmuradov/solana-bot/internal/types"
	"github.com/rovshanmuradov/solana-bot/internal/wallet"
	"go.uber.org/zap"
)

type RaydiumDEX struct {
	client   *solanaClient.Client
	logger   *zap.Logger
	poolInfo *RaydiumPoolInfo
}

func NewRaydiumDEX(client *solanaClient.Client, logger *zap.Logger, poolInfo *RaydiumPoolInfo) *RaydiumDEX {
	return &RaydiumDEX{
		client:   client,
		logger:   logger,
		poolInfo: poolInfo,
	}
}

func (r *RaydiumDEX) Name() string {
	return "Raydium"
}

func (r *RaydiumDEX) PrepareSwapInstruction(
	ctx context.Context,
	wallet solana.PublicKey,
	sourceToken solana.PublicKey,
	destinationToken solana.PublicKey,
	amountIn uint64,
	minAmountOut uint64,
	logger *zap.Logger,
) (solana.Instruction, error) {
	instruction, err := r.CreateSwapInstruction(
		wallet,
		sourceToken,
		destinationToken,
		amountIn,
		minAmountOut,
		logger,
		r.poolInfo,
	)
	if err != nil {
		return nil, err
	}
	return instruction, nil
}

// Добавляем метод для выполнения транзакции
func (r *RaydiumDEX) ExecuteSwap(
	ctx context.Context,
	task *types.Task,
	wallet *wallet.Wallet,
) error {
	return r.PrepareAndSendTransaction(ctx, task, wallet, r.logger)
}
