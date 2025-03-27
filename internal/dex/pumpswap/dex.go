// =============================
// File: internal/dex/pumpswap/dex.go
// =============================
package pumpswap

import (
	"context"
	"fmt"
	"github.com/gagliardetto/solana-go"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc"
	"github.com/rovshanmuradov/solana-bot/internal/wallet"
	"go.uber.org/zap"
	"math"
	"math/big"
	"time"
)

// NewDEX создаёт новый экземпляр DEX для PumpSwap.
func NewDEX(
	client *solbc.Client,
	w *wallet.Wallet,
	logger *zap.Logger,
	config *Config,
	poolManager PoolManagerInterface, // теперь передаём интерфейс
	monitorInterval string,
) (*DEX, error) {
	if client == nil || w == nil || logger == nil || config == nil || poolManager == nil {
		return nil, fmt.Errorf("client, wallet, logger, poolManager и config не могут быть nil")
	}
	if monitorInterval != "" {
		config.MonitorInterval = monitorInterval
	}
	return &DEX{
		client:      client,
		wallet:      w,
		logger:      logger,
		config:      config,
		poolManager: poolManager,
	}, nil
}

// ExecuteSwap выполняет операцию обмена на DEX.
//
// Принимает SwapParams, содержащий следующие параметры:
// - IsBuy: для покупки (true) выполняется инструкция buy, для продажи (false) - sell
// - Amount: количество токенов в базовых единицах (для продажи) или SOL в лампортах (для покупки)
// - SlippagePercent: допустимое проскальзывание в процентах (0-100)
// - PriorityFeeSol: приоритетная комиссия в SOL (строковое представление)
// - ComputeUnits: лимит вычислительных единиц для транзакции
//
// Возвращает ошибку в случае неудачи, включая специализированную SlippageExceededError
// при превышении допустимого проскальзывания.
func (d *DEX) ExecuteSwap(ctx context.Context, params SwapParams) error {
	pool, _, err := d.findAndValidatePool(ctx)
	if err != nil {
		return err
	}

	accounts, err := d.prepareTokenAccounts(ctx, pool)
	if err != nil {
		return err
	}

	priorityInstructions, err := d.preparePriorityInstructions(params.ComputeUnits, params.PriorityFeeSol)
	if err != nil {
		return err
	}

	// Вычисляем параметры для свапа
	amounts := d.calculateSwapAmounts(pool, params.IsBuy, params.Amount, params.SlippagePercent)

	// Создаем и отправляем транзакцию
	instructions := d.buildSwapTransaction(pool, accounts, params.IsBuy, amounts.BaseAmount,
		amounts.QuoteAmount, priorityInstructions)

	sig, err := d.buildAndSubmitTransaction(ctx, instructions)
	if err != nil {
		return d.handleSwapError(err, params)
	}

	d.logger.Info("Swap executed successfully",
		zap.String("signature", sig.String()),
		zap.Bool("is_buy", params.IsBuy),
		zap.Uint64("amount", params.Amount))
	return nil
}

// Новый метод для подготовки инструкций
func (d *DEX) prepareSwapInstructions(pool *PoolInfo, accounts *PreparedTokenAccounts,
	params SwapParams, amounts *SwapAmounts) ([]solana.Instruction, error) {

	priorityInstructions, err := d.preparePriorityInstructions(params.ComputeUnits, params.PriorityFeeSol)
	if err != nil {
		return nil, err
	}

	return d.buildSwapTransaction(pool, accounts, params.IsBuy, amounts.BaseAmount,
		amounts.QuoteAmount, priorityInstructions), nil
}

// ExecuteSell выполняет операцию продажи токена за WSOL
func (d *DEX) ExecuteSell(ctx context.Context, tokenAmount uint64, slippagePercent float64, priorityFeeSol string, computeUnits uint32) error {
	params := SwapParams{
		IsBuy:           false,
		Amount:          tokenAmount,
		SlippagePercent: slippagePercent,
		PriorityFeeSol:  priorityFeeSol,
		ComputeUnits:    computeUnits,
	}
	return d.ExecuteSwap(ctx, params)
}

// GetTokenPrice вычисляет цену токена по соотношению резервов пула.
func (d *DEX) GetTokenPrice(ctx context.Context, tokenMint string) (float64, error) {
	// Здесь мы считаем, что tokenMint должен соответствовать effectiveBaseMint.
	effBase, effQuote := d.effectiveMints()
	mint, err := solana.PublicKeyFromBase58(tokenMint)
	if err != nil {
		return 0, fmt.Errorf("invalid token mint: %w", err)
	}
	if !mint.Equals(effBase) {
		return 0, fmt.Errorf("token mint mismatch: expected %s, got %s", effBase.String(), mint.String())
	}
	pool, err := d.poolManager.FindPoolWithRetry(ctx, effBase, effQuote, 3, 1*time.Second)
	if err != nil {
		return 0, fmt.Errorf("failed to find pool: %w", err)
	}
	var price float64
	if pool.BaseReserves > 0 && pool.QuoteReserves > 0 {
		solDecimals := uint8(WSOLDecimals)
		tokenDecimals := d.getTokenDecimals(ctx, pool.BaseMint, DefaultTokenDecimals)
		baseReserves := new(big.Float).SetUint64(pool.BaseReserves)
		quoteReserves := new(big.Float).SetUint64(pool.QuoteReserves)
		ratio := new(big.Float).Quo(baseReserves, quoteReserves)
		adjustment := math.Pow10(int(tokenDecimals)) / math.Pow10(int(solDecimals))
		adjustedRatio := new(big.Float).Mul(ratio, big.NewFloat(adjustment))
		price, _ = adjustedRatio.Float64()
	}
	return price, nil
}
