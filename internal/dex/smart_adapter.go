// =============================================
// File: internal/dex/smart_adapter.go
// =============================================
package dex

import (
	"context"
	"fmt"
	"go.uber.org/zap"
	"strings"

	"github.com/rovshanmuradov/solana-bot/internal/dex/model"
	"github.com/rovshanmuradov/solana-bot/internal/task"
)

type smartDEXAdapter struct {
	baseDEXAdapter
	pumpfunAdapter  *pumpfunDEXAdapter
	pumpswapAdapter *pumpswapDEXAdapter
	// выбранный DEX: pumpfun или pumpswap
	dex DEX
}

func (d *smartDEXAdapter) Execute(ctx context.Context, t *task.Task) error {
	if t.TokenMint == "" {
		return fmt.Errorf("token mint is required")
	}
	// один раз выбираем DEX
	if d.dex == nil {
		dex, err := d.determineDEX(ctx, t.TokenMint)
		if err != nil {
			return fmt.Errorf("determine DEX: %w", err)
		}
		d.dex = dex
	}

	// проксируем tokenMint в базовом адаптере
	d.mu.Lock()
	d.tokenMint = t.TokenMint
	d.mu.Unlock()

	// готовим таск
	adaptedTask := *t
	if d.dex == d.pumpfunAdapter {
		adaptedTask.Operation = task.OperationSnipe
		d.logger.Info("Using Pump.fun for snipe", zap.String("token", t.TokenMint))
	} else {
		adaptedTask.Operation = task.OperationSwap
		d.logger.Info("Using Pump.swap for swap", zap.String("token", t.TokenMint))
	}

	// выполняем
	err := d.dex.Execute(ctx, &adaptedTask)
	// fallback по AnchorError 6005 (BondingCurveComplete)
	if isBondingCurveCompleteError(err) && d.dex == d.pumpfunAdapter {
		d.logger.Info("Caught BondingCurveComplete, falling back to Pump.swap", zap.String("token", t.TokenMint))
		d.dex = d.pumpswapAdapter
		adaptedTask.Operation = task.OperationSwap
		return d.dex.Execute(ctx, &adaptedTask)
	}
	return err
}

func (d *smartDEXAdapter) determineDEX(ctx context.Context, tokenMint string) (DEX, error) {
	if d.pumpfunAdapter == nil {
		d.pumpfunAdapter = &pumpfunDEXAdapter{
			baseDEXAdapter: baseDEXAdapter{
				client: d.client,
				wallet: d.wallet,
				logger: d.logger.Named("pumpfun"),
				name:   "Pump.fun",
			},
		}
	}
	if d.pumpswapAdapter == nil {
		d.pumpswapAdapter = &pumpswapDEXAdapter{
			baseDEXAdapter: baseDEXAdapter{
				client: d.client,
				wallet: d.wallet,
				logger: d.logger.Named("pumpswap"),
				name:   "Pump.Swap",
			},
		}
	}

	if err := d.pumpfunAdapter.init(ctx, tokenMint, d.pumpfunAdapter.makeInitPumpFun(tokenMint)); err != nil {
		d.logger.Warn("Failed to initialize PumpFun adapter, falling back to PumpSwap",
			zap.String("token_mint", tokenMint),
			zap.Error(err))
		return d.pumpswapAdapter, fmt.Errorf("pumpfun initialization failed: %w", err)
	}
	if d.pumpfunAdapter.inner == nil {
		return d.pumpswapAdapter, nil
	}

	isComplete, err := d.pumpfunAdapter.inner.IsBondingCurveComplete(ctx)
	if err != nil {
		d.logger.Warn("Failed to check if bonding curve is complete, falling back to PumpSwap",
			zap.String("token_mint", tokenMint),
			zap.Error(err))
		return d.pumpswapAdapter, fmt.Errorf("checking bonding curve status failed: %w", err)
	}
	if isComplete {
		return d.pumpswapAdapter, nil
	}
	return d.pumpfunAdapter, nil
}

func isBondingCurveCompleteError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "BondingCurveComplete")
}

func (d *smartDEXAdapter) GetTokenPrice(ctx context.Context, tokenMint string) (float64, error) {
	d.mu.Lock()
	d.tokenMint = tokenMint
	d.mu.Unlock()
	if d.dex == nil {
		return 0, fmt.Errorf("DEX not initialized")
	}
	return d.dex.GetTokenPrice(ctx, tokenMint)
}

func (d *smartDEXAdapter) GetTokenBalance(ctx context.Context, tokenMint string) (uint64, error) {
	d.mu.Lock()
	d.tokenMint = tokenMint
	d.mu.Unlock()
	if d.dex == nil {
		return 0, fmt.Errorf("DEX not initialized")
	}
	return d.dex.GetTokenBalance(ctx, tokenMint)
}

func (d *smartDEXAdapter) SellPercentTokens(ctx context.Context, tokenMint string, pct, slip float64, fee string, cu uint32) error {
	d.mu.Lock()
	d.tokenMint = tokenMint
	d.mu.Unlock()
	if d.dex == nil {
		return fmt.Errorf("DEX not initialized")
	}
	return d.dex.SellPercentTokens(ctx, tokenMint, pct, slip, fee, cu)
}

func (d *smartDEXAdapter) CalculatePnL(ctx context.Context, amount, invest float64) (*model.PnLResult, error) {
	d.mu.Lock()
	tokenMint := d.tokenMint
	d.mu.Unlock()
	if tokenMint == "" {
		return nil, fmt.Errorf("token mint is not set")
	}
	if d.dex == nil {
		return nil, fmt.Errorf("DEX not initialized")
	}
	return d.dex.CalculatePnL(ctx, amount, invest)
}
