// =============================
// File: internal/dex/pumpswap_adapter.go
// =============================
package dex

import (
	"context"
	"fmt"
	"github.com/gagliardetto/solana-go"
	"github.com/rovshanmuradov/solana-bot/internal/dex/pumpswap"
	"go.uber.org/zap"
	"math"
)

// GetName для PumpSwap DEX
func (d *pumpswapDEXAdapter) GetName() string {
	return "Pump.Swap"
}

// GetTokenPrice для Pump.swap DEX
func (d *pumpswapDEXAdapter) GetTokenPrice(ctx context.Context, tokenMint string) (float64, error) {
	if err := d.initPumpSwap(ctx, tokenMint); err != nil {
		return 0, err
	}
	return d.inner.GetTokenPrice(ctx, tokenMint)
}

// Execute выполняет операцию на Pump.swap DEX
func (d *pumpswapDEXAdapter) Execute(ctx context.Context, task *Task) error {
	if task.TokenMint == "" {
		return fmt.Errorf("token mint address is required for Pump.swap")
	}

	if err := d.initPumpSwap(ctx, task.TokenMint); err != nil {
		return err
	}

	switch task.Operation {
	case OperationSwap:
		d.logger.Info("Executing swap on Pump.swap",
			zap.String("token_mint", task.TokenMint),
			zap.Float64("amount_sol", task.AmountSol),
			zap.Float64("slippage_percent", task.SlippagePercent),
			zap.String("priority_fee", task.PriorityFee),
			zap.Uint32("compute_units", task.ComputeUnits))

		amountLamports := uint64(task.AmountSol * 1e9)

		return d.inner.ExecuteSwap(ctx, pumpswap.SwapParams{
			IsBuy:           true,
			Amount:          amountLamports,
			SlippagePercent: task.SlippagePercent,
			PriorityFeeSol:  task.PriorityFee,
			ComputeUnits:    task.ComputeUnits,
		})

	case OperationSell:
		tokenMintPubkey, err := solana.PublicKeyFromBase58(task.TokenMint)
		if err != nil {
			return fmt.Errorf("invalid token mint address: %w", err)
		}

		precision, err := d.inner.DetermineTokenPrecision(ctx, tokenMintPubkey)
		if err != nil {
			precision = 6
			d.logger.Warn("Could not determine token precision, using default",
				zap.Uint8("default_precision", precision))
		}

		tokenAmount := uint64(task.AmountSol * math.Pow(10, float64(precision)))

		d.logger.Info("Executing sell on Pump.swap",
			zap.String("token_mint", task.TokenMint),
			zap.Float64("tokens_to_sell", task.AmountSol),
			zap.Uint64("token_amount", tokenAmount),
			zap.Float64("slippage_percent", task.SlippagePercent),
			zap.String("priority_fee", task.PriorityFee),
			zap.Uint32("compute_units", task.ComputeUnits))

		return d.inner.ExecuteSell(ctx, tokenAmount, task.SlippagePercent, task.PriorityFee, task.ComputeUnits)

	default:
		return fmt.Errorf("operation %s is not supported on Pump.swap", task.Operation)
	}
}

// initPumpSwap инициализирует Pump.swap DEX если необходимо
func (d *pumpswapDEXAdapter) initPumpSwap(_ context.Context, tokenMint string) error {
	d.initMu.Lock()
	defer d.initMu.Unlock()

	if d.initDone && d.tokenMint == tokenMint && d.inner != nil {
		return nil
	}

	config := pumpswap.GetDefaultConfig()
	if err := config.SetupForToken(tokenMint, d.logger); err != nil {
		return fmt.Errorf("failed to setup Pump.swap configuration: %w", err)
	}

	poolManager := pumpswap.NewPoolManager(d.client, d.logger)

	var err error
	d.inner, err = pumpswap.NewDEX(d.client, d.wallet, d.logger, config, poolManager, config.MonitorInterval)
	if err != nil {
		return fmt.Errorf("failed to initialize Pump.swap DEX: %w", err)
	}

	d.tokenMint = tokenMint
	d.initDone = true
	return nil
}

// GetTokenBalance возвращает текущий баланс токена
// Placeholder для совместимости с интерфейсом DEX
func (d *pumpswapDEXAdapter) GetTokenBalance(ctx context.Context, tokenMint string) (uint64, error) {
	// В будущем здесь можно реализовать настоящую логику получения баланса
	d.logger.Debug("GetTokenBalance called on PumpSwap (not fully implemented)",
		zap.String("token_mint", tokenMint))

	// Возможно реализовать в будущем, сейчас просто возвращаем ошибку
	return 0, fmt.Errorf("GetTokenBalance not fully implemented for Pump.Swap DEX")
}

// SellPercentTokens продает указанный процент имеющихся токенов
func (d *pumpswapDEXAdapter) SellPercentTokens(ctx context.Context, tokenMint string, percentToSell float64, slippagePercent float64, priorityFeeSol string, computeUnits uint32) error {
	if err := d.initPumpSwap(ctx, tokenMint); err != nil {
		return err
	}

	d.logger.Warn("SellPercentTokens is not fully implemented for PumpSwap",
		zap.String("token_mint", tokenMint),
		zap.Float64("percent_to_sell", percentToSell))

	// Получаем баланс токена (в настоящее время не реализовано полностью)
	balance, err := d.inner.GetTokenBalance(ctx, tokenMint)
	if err != nil {
		return fmt.Errorf("failed to get token balance: %w", err)
	}

	// Рассчитываем количество токенов для продажи
	tokensToSell := uint64(float64(balance) * percentToSell / 100.0)

	// Выполняем стандартную операцию продажи
	return d.inner.ExecuteSell(ctx, tokensToSell, slippagePercent, priorityFeeSol, computeUnits)
}
