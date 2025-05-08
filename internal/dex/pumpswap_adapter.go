// internal/dex/pumpswap_adapter.go
package dex

import (
	"context"
	"fmt"
	"github.com/rovshanmuradov/solana-bot/internal/dex/model"
	"math"

	"github.com/gagliardetto/solana-go"
	"go.uber.org/zap"

	"github.com/rovshanmuradov/solana-bot/internal/dex/pumpswap"
)

// pumpswapDEXAdapter адаптирует Pump.Swap к нашему DEX-интерфейсу.
type pumpswapDEXAdapter struct {
	baseDEXAdapter
	inner *pumpswap.DEX
}

// Execute выполняет swap или sell, обеспечивая ленивую инициализацию через initIfNeeded.
func (d *pumpswapDEXAdapter) Execute(ctx context.Context, task *Task) error {
	if task.TokenMint == "" {
		return fmt.Errorf("token mint is required for Pump.swap")
	}

	// Ленивая инициализация
	if err := d.initIfNeeded(ctx, task.TokenMint, func() error {
		cfg := pumpswap.GetDefaultConfig()
		if err := cfg.SetupForToken(task.TokenMint, d.logger); err != nil {
			return fmt.Errorf("setup Pump.swap config: %w", err)
		}
		pm := pumpswap.NewPoolManager(d.client, d.logger)
		var err error
		d.inner, err = pumpswap.NewDEX(d.client, d.wallet, d.logger, cfg, pm, cfg.MonitorInterval)
		return err
	}); err != nil {
		return err
	}

	switch task.Operation {
	case OperationSwap:
		lamports := uint64(task.AmountSol * 1e9)
		d.logger.Info("Executing swap on Pump.swap",
			zap.String("token_mint", task.TokenMint),
			zap.Uint64("lamports", lamports),
			zap.Float64("slippage_percent", task.SlippagePercent),
			zap.String("priority_fee", task.PriorityFee),
			zap.Uint32("compute_units", task.ComputeUnits),
		)
		return d.inner.ExecuteSwap(ctx, pumpswap.SwapParams{
			IsBuy:           true,
			Amount:          lamports,
			SlippagePercent: task.SlippagePercent,
			PriorityFeeSol:  task.PriorityFee,
			ComputeUnits:    task.ComputeUnits,
		})

	case OperationSell:
		mintPub, err := solana.PublicKeyFromBase58(task.TokenMint)
		if err != nil {
			return fmt.Errorf("invalid token mint: %w", err)
		}
		precision, err := d.inner.DetermineTokenPrecision(ctx, mintPub)
		if err != nil {
			precision = 6
			d.logger.Warn("Using default precision", zap.Uint8("precision", precision))
		}
		amount := uint64(task.AmountSol * math.Pow(10, float64(precision)))
		d.logger.Info("Executing sell on Pump.swap",
			zap.String("token_mint", task.TokenMint),
			zap.Uint64("amount", amount),
		)
		return d.inner.ExecuteSell(ctx, amount, task.SlippagePercent, task.PriorityFee, task.ComputeUnits)

	default:
		return fmt.Errorf("operation %s is not supported on Pump.swap", task.Operation)
	}
}

// GetTokenBalance возвращает баланс, предварительно инициализировав DEX.
func (d *pumpswapDEXAdapter) GetTokenBalance(ctx context.Context, tokenMint string) (uint64, error) {
	if err := d.initIfNeeded(ctx, tokenMint, d.makeInitPumpSwap()); err != nil {
		return 0, fmt.Errorf("init Pump.swap: %w", err)
	}
	return d.inner.GetTokenBalance(ctx)
}

// SellPercentTokens продаёт процент токенов, предварительно инициализировав DEX.
func (d *pumpswapDEXAdapter) SellPercentTokens(ctx context.Context, tokenMint string, percentToSell, slippage float64, priorityFee string, computeUnits uint32) error {
	if err := d.initIfNeeded(ctx, tokenMint, d.makeInitPumpSwap()); err != nil {
		return fmt.Errorf("init Pump.swap: %w", err)
	}
	return d.inner.SellPercentTokens(ctx, percentToSell, slippage, priorityFee, computeUnits)
}

// GetTokenPrice возвращает цену, предварительно инициализировав DEX.
func (d *pumpswapDEXAdapter) GetTokenPrice(ctx context.Context, tokenMint string) (float64, error) {
	if err := d.initIfNeeded(ctx, tokenMint, d.makeInitPumpSwap()); err != nil {
		return 0, fmt.Errorf("init Pump.swap: %w", err)
	}
	return d.inner.GetTokenPrice(ctx, tokenMint)
}

// CalculatePnL рассчитывает PnL, предварительно инициализировав DEX.
func (d *pumpswapDEXAdapter) CalculatePnL(ctx context.Context, tokenAmount, initialInvestment float64) (*model.PnLResult, error) {
	if err := d.initIfNeeded(ctx, d.tokenMint, d.makeInitPumpSwap()); err != nil {
		return nil, err
	}
	return d.inner.CalculatePnL(ctx, tokenAmount, initialInvestment)
}

// Вспомогательный метод для передачи initFn
func (d *pumpswapDEXAdapter) makeInitPumpSwap() func() error {
	return func() error {
		cfg := pumpswap.GetDefaultConfig()
		if err := cfg.SetupForToken(d.tokenMint, d.logger); err != nil {
			return fmt.Errorf("setup Pump.swap: %w", err)
		}
		pm := pumpswap.NewPoolManager(d.client, d.logger)
		var err error
		d.inner, err = pumpswap.NewDEX(d.client, d.wallet, d.logger, cfg, pm, cfg.MonitorInterval)
		return err
	}
}
