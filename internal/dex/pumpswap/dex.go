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
	"time"
)

// NewDEX создаёт новый экземпляр DEX для PumpSwap с кешированием и отложенным логированием RPC.
func NewDEX(
	client *solbc.Client,
	w *wallet.Wallet,
	logger *zap.Logger,
	config *Config,
	poolManager PoolManagerInterface,
	monitorInterval string,
) (*DEX, error) {
	if client == nil || w == nil || logger == nil || config == nil || poolManager == nil {
		return nil, fmt.Errorf("client, wallet, logger, poolManager и config не могут быть nil")
	}
	if monitorInterval != "" {
		config.MonitorInterval = monitorInterval
	}

	d := &DEX{
		client:      client,
		wallet:      w,
		logger:      logger,
		config:      config,
		poolManager: poolManager,
		// Устанавливаем TTL кеша для пулов и цен
		cacheValidPeriod: 10 * time.Second,
	}
	return d, nil
}

// ExecuteSwap выполняет операцию обмена на DEX.
func (d *DEX) ExecuteSwap(ctx context.Context, params SwapParams) error {
	pool, _, err := d.findAndValidatePool(ctx)
	if err != nil {
		return err
	}

	accounts, err := d.prepareTokenAccounts(ctx, pool)
	if err != nil {
		return err
	}

	// Вычисляем параметры для свапа
	amounts := d.calculateSwapAmounts(pool, params.IsBuy, params.Amount, params.SlippagePercent)

	// Подготавливаем инструкции для транзакции
	instructions, err := d.prepareSwapInstructions(pool, accounts, params, amounts)
	if err != nil {
		return err
	}

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

// prepareSwapInstructions подготавливает инструкции для выполнения операции свапа.
func (d *DEX) prepareSwapInstructions(pool *PoolInfo, accounts *PreparedTokenAccounts,
	params SwapParams, amounts *SwapAmounts) ([]solana.Instruction, error) {

	priorityInstructions, err := d.preparePriorityInstructions(params.ComputeUnits, params.PriorityFeeSol)
	if err != nil {
		return nil, err
	}

	return d.buildSwapTransaction(pool, accounts, params.IsBuy, amounts.BaseAmount,
		amounts.QuoteAmount, priorityInstructions), nil
}

// ExecuteSell выполняет операцию продажи токена за WSOL.
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
