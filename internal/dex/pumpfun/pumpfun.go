// =============================
// File: internal/dex/pumpfun/pumpfun.go
// =============================
package pumpfun

import (
	"context"
	"fmt"
	"time"

	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc"
	"github.com/rovshanmuradov/solana-bot/internal/types"
	"github.com/rovshanmuradov/solana-bot/internal/wallet"
	"go.uber.org/zap"
)

// DEX is the Pump.fun DEX implementation
type DEX struct {
	client          *solbc.Client
	wallet          *wallet.Wallet
	logger          *zap.Logger
	config          *Config
	priorityManager *types.PriorityManager
}

// NewDEX creates a new instance of the Pump.fun DEX
func NewDEX(client *solbc.Client, w *wallet.Wallet, logger *zap.Logger, config *Config, _ string) (*DEX, error) {
	if config.ContractAddress.IsZero() {
		return nil, fmt.Errorf("pump.fun contract address is required")
	}
	if config.Mint.IsZero() {
		return nil, fmt.Errorf("token mint address is required")
	}

	logger.Info("Creating PumpFun DEX",
		zap.String("contract", config.ContractAddress.String()),
		zap.String("token_mint", config.Mint.String()))

	dex := &DEX{
		client:          client,
		wallet:          w,
		logger:          logger.Named("pumpfun"),
		config:          config,
		priorityManager: types.NewPriorityManager(logger.Named("priority")),
	}

	// Update fee recipient
	fetchCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	globalAccount, err := FetchGlobalAccount(fetchCtx, client, config.Global)
	if err != nil {
		logger.Warn("Failed to fetch global account data, using default fee recipient",
			zap.Error(err))
	} else if globalAccount != nil {
		config.FeeRecipient = globalAccount.FeeRecipient
		logger.Info("Updated fee recipient", zap.String("address", config.FeeRecipient.String()))
	}

	return dex, nil
}

// ExecuteSnipe executes a buy operation using exact-sol program
func (d *DEX) ExecuteSnipe(ctx context.Context, amountSol float64, slippagePercent float64, priorityFeeSol string, computeUnits uint32) error {
	d.logger.Info("Starting Pump.fun exact-sol buy operation",
		zap.Float64("amount_sol", amountSol),
		zap.Float64("slippage_percent", slippagePercent),
		zap.String("priority_fee_sol", priorityFeeSol),
		zap.Uint32("compute_units", computeUnits))

	// Create context with timeout
	opCtx, cancel := d.prepareTransactionContext(ctx, 45*time.Second)
	defer cancel()

	// Convert SOL amount to lamports
	solAmountLamports := uint64(amountSol * 1_000_000_000)

	d.logger.Info("Using exact SOL amount",
		zap.Uint64("sol_amount_lamports", solAmountLamports),
		zap.String("sol_amount", fmt.Sprintf("%.9f SOL", float64(solAmountLamports)/1_000_000_000)))

	// Prepare buy transaction
	instructions, err := d.prepareBuyTransaction(opCtx, solAmountLamports, priorityFeeSol, computeUnits)
	if err != nil {
		return err
	}

	// Send and confirm transaction
	_, err = d.sendAndConfirmTransaction(opCtx, instructions)
	return err
}

// ExecuteSell executes a sell operation
func (d *DEX) ExecuteSell(ctx context.Context, tokenAmount uint64, slippagePercent float64, priorityFeeSol string, computeUnits uint32) error {
	d.logger.Info("Starting Pump.fun sell operation",
		zap.Uint64("token_amount", tokenAmount),
		zap.Float64("slippage_percent", slippagePercent),
		zap.String("priority_fee_sol", priorityFeeSol),
		zap.Uint32("compute_units", computeUnits))

	// Create context with timeout
	opCtx, cancel := d.prepareTransactionContext(ctx, 45*time.Second)
	defer cancel()

	// Prepare sell transaction
	instructions, err := d.prepareSellTransaction(opCtx, tokenAmount, slippagePercent, priorityFeeSol, computeUnits)
	if err != nil {
		return err
	}

	// Send and confirm transaction
	_, err = d.sendAndConfirmTransaction(opCtx, instructions)
	if err != nil {
		return d.handleSellError(err)
	}

	return nil
}
