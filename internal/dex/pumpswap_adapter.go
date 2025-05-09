// internal/dex/pumpswap_adapter.go
package dex

import (
	"context"
	"fmt"
	"github.com/rovshanmuradov/solana-bot/internal/dex/model"
	"github.com/rovshanmuradov/solana-bot/internal/task"
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
func (d *pumpswapDEXAdapter) Execute(ctx context.Context, t *task.Task) error {
	if t.TokenMint == "" {
		return fmt.Errorf("token mint is required for Pump.swap")
	}
	if err := d.init(ctx, t.TokenMint, d.makeInitPumpSwap(t.TokenMint)); err != nil {
		return err
	}

	switch t.Operation {
	case task.OperationSwap:
		lamports := uint64(t.AmountSol * 1e9)
		d.logger.Info("Executing swap on Pump.swap",
			zap.String("token_mint", t.TokenMint),
			zap.Uint64("lamports", lamports),
			zap.Float64("slippage_percent", t.SlippagePercent),
			zap.String("priority_fee", t.PriorityFeeSol),
			zap.Uint32("compute_units", t.ComputeUnits),
		)
		return d.inner.ExecuteSwap(ctx, pumpswap.SwapParams{
			IsBuy:           true,
			Amount:          lamports,
			SlippagePercent: t.SlippagePercent,
			PriorityFeeSol:  t.PriorityFeeSol,
			ComputeUnits:    t.ComputeUnits,
		})

	case task.OperationSell:
		mintPub, err := solana.PublicKeyFromBase58(t.TokenMint)
		if err != nil {
			return fmt.Errorf("invalid token mint: %w", err)
		}
		precision, err := d.inner.DetermineTokenPrecision(ctx, mintPub)
		if err != nil {
			precision = 6
			d.logger.Warn("Using default precision", zap.Uint8("precision", precision))
		}
		amount := uint64(t.AmountSol * math.Pow(10, float64(precision)))
		d.logger.Info("Executing sell on Pump.swap",
			zap.String("token_mint", t.TokenMint),
			zap.Uint64("amount", amount),
		)
		return d.inner.ExecuteSell(ctx, amount, t.SlippagePercent, t.PriorityFeeSol, t.ComputeUnits)

	default:
		return fmt.Errorf("operation %s is not supported on Pump.swap", t.Operation)
	}
}

// GetTokenBalance возвращает баланс, предварительно инициализировав DEX.
func (d *pumpswapDEXAdapter) GetTokenBalance(ctx context.Context, tokenMint string) (uint64, error) {
	if err := d.init(ctx, tokenMint, d.makeInitPumpSwap(tokenMint)); err != nil {
		return 0, fmt.Errorf("init Pump.swap: %w", err)
	}
	return d.inner.GetTokenBalance(ctx)
}

// SellPercentTokens продаёт процент токенов, предварительно инициализировав DEX.
func (d *pumpswapDEXAdapter) SellPercentTokens(ctx context.Context, tokenMint string, percentToSell, slippage float64, priorityFee string, computeUnits uint32) error {
	if err := d.init(ctx, tokenMint, d.makeInitPumpSwap(tokenMint)); err != nil {
		return fmt.Errorf("init Pump.swap: %w", err)
	}
	return d.inner.SellPercentTokens(ctx, percentToSell, slippage, priorityFee, computeUnits)
}

// GetTokenPrice возвращает цену, предварительно инициализировав DEX.
func (d *pumpswapDEXAdapter) GetTokenPrice(ctx context.Context, tokenMint string) (float64, error) {
	if err := d.init(ctx, tokenMint, d.makeInitPumpSwap(tokenMint)); err != nil {
		return 0, fmt.Errorf("init Pump.swap: %w", err)
	}
	return d.inner.GetTokenPrice(ctx, tokenMint)
}

// CalculatePnL рассчитывает PnL, предварительно инициализировав DEX.
func (d *pumpswapDEXAdapter) CalculatePnL(ctx context.Context, tokenAmount, initialInvestment float64) (*model.PnLResult, error) {
	d.mu.Lock()
	tokenMint := d.tokenMint
	d.mu.Unlock()

	if err := d.init(ctx, tokenMint, d.makeInitPumpSwap(tokenMint)); err != nil {
		return nil, err
	}
	return d.inner.CalculatePnL(ctx, tokenAmount, initialInvestment)
}

// Вспомогательный метод для передачи initFn
func (d *pumpswapDEXAdapter) makeInitPumpSwap(tokenMint string) func() error {
	return func() error {
		cfg := pumpswap.GetDefaultConfig()
		if err := cfg.SetupForToken(tokenMint, d.logger); err != nil {
			return fmt.Errorf("setup Pump.swap: %w", err)
		}
		pm := pumpswap.NewPoolManager(d.client, d.logger)
		var err error
		d.inner, err = pumpswap.NewDEX(d.client, d.wallet, d.logger, cfg, pm, cfg.MonitorInterval)
		return err
	}
}
