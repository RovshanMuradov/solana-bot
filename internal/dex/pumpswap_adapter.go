// =============================
// File: internal/dex/pumpswap_adapter.go
// =============================
package dex

import (
	"context"
	"fmt"
	"github.com/gagliardetto/solana-go"
	"github.com/rovshanmuradov/solana-bot/internal/dex/model"
	"github.com/rovshanmuradov/solana-bot/internal/dex/pumpswap"
	"go.uber.org/zap"
	"math"
)

// GetName возвращает название DEX.
func (d *pumpswapDEXAdapter) GetName() string {
	return "Pump.Swap"
}

// pumpswapDEXAdapter адаптирует Pump.Swap к общему интерфейсу DEX.
type pumpswapDEXAdapter struct {
	baseDEXAdapter
	inner *pumpswap.DEX
}

// Execute выполняет операцию на Pump.Swap DEX.
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
		// Проверяем, указан ли специальный процент для продажи
		if task.SellPercentage > 0 {
			// Если указан процент продажи, используем метод SellPercentTokens
			d.logger.Info("Executing percent-based sell on Pump.swap",
				zap.String("token_mint", task.TokenMint),
				zap.Float64("percent_to_sell", task.SellPercentage),
				zap.Float64("slippage_percent", task.SlippagePercent),
				zap.String("priority_fee", task.PriorityFee),
				zap.Uint32("compute_units", task.ComputeUnits))

			return d.SellPercentTokens(ctx, task.TokenMint, task.SellPercentage,
				task.SlippagePercent, task.PriorityFee, task.ComputeUnits)
		}

		// Стандартная продажа с указанным количеством токенов
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

// initPumpSwap инициализирует адаптер Pump.Swap DEX при необходимости.
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

// GetTokenBalance возвращает текущий баланс токена на аккаунте пользователя.
func (d *pumpswapDEXAdapter) GetTokenBalance(ctx context.Context, tokenMint string) (uint64, error) {
	if err := d.initPumpSwap(ctx, tokenMint); err != nil {
		return 0, fmt.Errorf("failed to initialize PumpSwap: %w", err)
	}
	return d.inner.GetTokenBalance(ctx)
}

// SellPercentTokens продает указанный процент имеющихся токенов.
func (d *pumpswapDEXAdapter) SellPercentTokens(ctx context.Context, tokenMint string, percentToSell float64,
	slippagePercent float64, priorityFeeSol string, computeUnits uint32) error {

	// Инициализация PumpSwap если необходимо
	if err := d.initPumpSwap(ctx, tokenMint); err != nil {
		return fmt.Errorf("failed to initialize PumpSwap: %w", err)
	}
	// Делегируем выполнение операции внутреннему DEX
	return d.inner.SellPercentTokens(ctx, percentToSell, slippagePercent, priorityFeeSol, computeUnits)
}

// GetTokenPrice получает текущую цену токена на Pump.Swap DEX.
func (d *pumpswapDEXAdapter) GetTokenPrice(ctx context.Context, tokenMint string) (float64, error) {
	if err := d.initPumpSwap(ctx, tokenMint); err != nil {
		return 0, err
	}
	return d.inner.GetTokenPrice(ctx, tokenMint)
}

func (d *pumpswapDEXAdapter) CalculatePnL(ctx context.Context, tokenAmount float64, initialInvestment float64) (*model.PnLResult, error) {
	if err := d.initPumpSwap(ctx, d.tokenMint); err != nil {
		return nil, err
	}
	return d.inner.CalculatePnL(ctx, tokenAmount, initialInvestment)
}
