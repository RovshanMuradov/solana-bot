package dex

import (
	"context"
	"fmt"
	"github.com/rovshanmuradov/solana-bot/internal/dex/pumpfun"
	"go.uber.org/zap"
)

// pumpfunDEXAdapter адаптирует Pump.fun к интерфейсу DEX
type pumpfunDEXAdapter struct {
	baseDEXAdapter
	inner *pumpfun.DEX
}

// GetName для PumpFun DEX
func (d *pumpfunDEXAdapter) GetName() string {
	return "Pump.fun"
}

// initPumpFun инициализирует Pump.fun DEX если необходимо
func (d *pumpfunDEXAdapter) initPumpFun(_ context.Context, tokenMint string) error {
	d.initMu.Lock()
	defer d.initMu.Unlock()

	if d.initDone && d.tokenMint == tokenMint && d.inner != nil {
		return nil
	}

	config := pumpfun.GetDefaultConfig()
	if err := config.SetupForToken(tokenMint, d.logger); err != nil {
		return fmt.Errorf("failed to setup Pump.fun configuration: %w", err)
	}

	var err error
	d.inner, err = pumpfun.NewDEX(d.client, d.wallet, d.logger, config, config.MonitorInterval)
	if err != nil {
		return fmt.Errorf("failed to initialize Pump.fun DEX: %w", err)
	}

	d.tokenMint = tokenMint
	d.initDone = true
	return nil
}

// Execute выполняет операцию на Pump.fun DEX
func (d *pumpfunDEXAdapter) Execute(ctx context.Context, task *Task) error {
	if task.TokenMint == "" {
		return fmt.Errorf("token mint address is required for Pump.fun")
	}

	if err := d.initPumpFun(ctx, task.TokenMint); err != nil {
		return err
	}

	switch task.Operation {
	case OperationSnipe:
		d.logger.Info("Executing snipe on Pump.fun",
			zap.String("token_mint", task.TokenMint),
			zap.Float64("sol_amount", task.AmountSol),
			zap.Float64("slippage_percent", task.SlippagePercent),
			zap.String("priority_fee", task.PriorityFee),
			zap.Uint32("compute_units", task.ComputeUnits))

		return d.inner.ExecuteSnipe(ctx, task.AmountSol, task.SlippagePercent, task.PriorityFee, task.ComputeUnits)

	case OperationSell:
		tokenAmount, err := convertToTokenUnits(ctx, d.inner, task.TokenMint, task.AmountSol, 6)
		if err != nil {
			return err
		}

		d.logger.Info("Executing sell on Pump.fun",
			zap.String("token_mint", task.TokenMint),
			zap.Float64("tokens_to_sell", task.AmountSol),
			zap.Uint64("token_amount", tokenAmount),
			zap.Float64("slippage_percent", task.SlippagePercent),
			zap.String("priority_fee", task.PriorityFee),
			zap.Uint32("compute_units", task.ComputeUnits))

		return d.inner.ExecuteSell(ctx, tokenAmount, task.SlippagePercent, task.PriorityFee, task.ComputeUnits)

	default:
		return fmt.Errorf("operation %s is not supported on Pump.fun", task.Operation)
	}
}

// GetTokenPrice для Pump.fun DEX
func (d *pumpfunDEXAdapter) GetTokenPrice(ctx context.Context, tokenMint string) (float64, error) {
	if err := d.initPumpFun(ctx, tokenMint); err != nil {
		return 0, err
	}
	return d.inner.GetTokenPrice(ctx, tokenMint)
}
