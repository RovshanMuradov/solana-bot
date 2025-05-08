// internal/dex/pumpfun_adapter.go
package dex

import (
	"context"
	"fmt"

	"github.com/gagliardetto/solana-go/rpc"
	"go.uber.org/zap"

	"github.com/rovshanmuradov/solana-bot/internal/dex/model"
	"github.com/rovshanmuradov/solana-bot/internal/dex/pumpfun"
)

// pumpfunDEXAdapter адаптирует Pump.fun к нашему DEX-интерфейсу.
type pumpfunDEXAdapter struct {
	baseDEXAdapter
	inner *pumpfun.DEX
}

// Execute выполняет snipe или sell, через общий initIfNeeded.
func (d *pumpfunDEXAdapter) Execute(ctx context.Context, task *Task) error {
	if task.TokenMint == "" {
		return fmt.Errorf("token mint is required for Pump.fun")
	}
	// ленивый init
	if err := d.initIfNeeded(ctx, task.TokenMint, d.makeInitPumpFun(task.TokenMint)); err != nil {
		return err
	}

	switch task.Operation {
	case OperationSnipe:
		d.logger.Info("Pump.fun snipe",
			zap.String("mint", task.TokenMint),
			zap.Float64("sol", task.AmountSol),
			zap.Float64("slippage", task.SlippagePercent),
			zap.String("fee", task.PriorityFee),
			zap.Uint32("cu", task.ComputeUnits),
		)
		return d.inner.ExecuteSnipe(ctx, task.AmountSol, task.SlippagePercent, task.PriorityFee, task.ComputeUnits)

	case OperationSell:
		bal, err := d.inner.GetTokenBalance(ctx, rpc.CommitmentConfirmed)
		if err != nil {
			return fmt.Errorf("get balance: %w", err)
		}
		if bal > 0 {
			return d.inner.ExecuteSell(ctx, bal, task.SlippagePercent, task.PriorityFee, task.ComputeUnits)
		}
		// fallback: конвертируем SOL в лампорты
		lamports := uint64(task.AmountSol * 1e9)
		return d.inner.ExecuteSell(ctx, lamports, task.SlippagePercent, task.PriorityFee, task.ComputeUnits)
	default:
		return fmt.Errorf("unsupported operation %s on Pump.fun", task.Operation)
	}
}

// GetTokenPrice возвращает цену, гарантируя init.
func (d *pumpfunDEXAdapter) GetTokenPrice(ctx context.Context, tokenMint string) (float64, error) {
	if err := d.initIfNeeded(ctx, tokenMint, d.makeInitPumpFun(tokenMint)); err != nil {
		return 0, err
	}
	return d.inner.GetTokenPrice(ctx, tokenMint)
}

// GetTokenBalance возвращает баланс, гарантируя init.
func (d *pumpfunDEXAdapter) GetTokenBalance(ctx context.Context, tokenMint string) (uint64, error) {
	if err := d.initIfNeeded(ctx, tokenMint, d.makeInitPumpFun(tokenMint)); err != nil {
		return 0, err
	}
	return d.inner.GetTokenBalance(ctx)
}

// SellPercentTokens продаёт процент, гарантируя init.
func (d *pumpfunDEXAdapter) SellPercentTokens(ctx context.Context, tokenMint string, pct, slip float64, fee string, cu uint32) error {
	if err := d.initIfNeeded(ctx, tokenMint, d.makeInitPumpFun(tokenMint)); err != nil {
		return err
	}
	return d.inner.SellPercentTokens(ctx, pct, slip, fee, cu)
}

// CalculatePnL считает PnL, гарантируя init.
func (d *pumpfunDEXAdapter) CalculatePnL(ctx context.Context, amount, invest float64) (*model.PnLResult, error) {
	if err := d.initIfNeeded(ctx, d.tokenMint, d.makeInitPumpFun(d.tokenMint)); err != nil {
		return nil, err
	}
	return d.inner.CalculatePnL(ctx, amount, invest)
}

// makeInitPumpFun возвращает initFn для initIfNeeded.
func (d *pumpfunDEXAdapter) makeInitPumpFun(tokenMint string) func() error {
	return func() error {
		cfg := pumpfun.GetDefaultConfig()
		if err := cfg.SetupForToken(tokenMint, d.logger); err != nil {
			return fmt.Errorf("setup Pump.fun config: %w", err)
		}
		var err error
		d.inner, err = pumpfun.NewDEX(d.client, d.wallet, d.logger, cfg, cfg.MonitorInterval)
		return err
	}
}
