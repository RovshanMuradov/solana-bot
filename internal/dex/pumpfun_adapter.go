// =============================
// File: internal/dex/pumpfun_adapter.go
// =============================
package dex

import (
	"context"
	"fmt"
	"github.com/gagliardetto/solana-go/rpc"
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
		// Получаем текущий баланс токенов для продажи всех имеющихся
		tokenBalance, err := d.inner.GetTokenBalance(ctx, rpc.CommitmentConfirmed)
		if err != nil {
			return fmt.Errorf("failed to get token balance for sell: %w", err)
		}

		// Если баланс получен успешно, используем его; иначе пытаемся конвертировать AmountSol в токены
		if tokenBalance > 0 {
			d.logger.Info("Executing sell on Pump.fun using actual token balance",
				zap.String("token_mint", task.TokenMint),
				zap.Uint64("token_balance", tokenBalance),
				zap.Float64("slippage_percent", task.SlippagePercent),
				zap.String("priority_fee", task.PriorityFee),
				zap.Uint32("compute_units", task.ComputeUnits))

			return d.inner.ExecuteSell(ctx, tokenBalance, task.SlippagePercent, task.PriorityFee, task.ComputeUnits)
		} else {
			// Запасной вариант, если не удалось получить баланс
			tokenAmount, err := convertToTokenUnits(ctx, d.inner, task.TokenMint, task.AmountSol, 6)
			if err != nil {
				return err
			}

			d.logger.Info("Executing sell on Pump.fun using converted amount",
				zap.String("token_mint", task.TokenMint),
				zap.Float64("tokens_to_sell", task.AmountSol),
				zap.Uint64("token_amount", tokenAmount),
				zap.Float64("slippage_percent", task.SlippagePercent),
				zap.String("priority_fee", task.PriorityFee),
				zap.Uint32("compute_units", task.ComputeUnits))

			return d.inner.ExecuteSell(ctx, tokenAmount, task.SlippagePercent, task.PriorityFee, task.ComputeUnits)
		}

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

// GetTokenBalance возвращает текущий баланс токена
func (d *pumpfunDEXAdapter) GetTokenBalance(ctx context.Context, tokenMint string) (uint64, error) {
	if err := d.initPumpFun(ctx, tokenMint); err != nil {
		return 0, fmt.Errorf("failed to initialize Pump.fun: %w", err)
	}

	return d.inner.GetTokenBalance(ctx)
}

// SellPercentTokens продает указанный процент имеющихся токенов
func (d *pumpfunDEXAdapter) SellPercentTokens(ctx context.Context, tokenMint string, percentToSell float64, slippagePercent float64, priorityFeeSol string, computeUnits uint32) error {
	if err := d.initPumpFun(ctx, tokenMint); err != nil {
		return err
	}

	return d.inner.SellPercentTokens(ctx, percentToSell, slippagePercent, priorityFeeSol, computeUnits)
}

// CalculateDiscretePnL вычисляет PnL с учетом дискретной структуры Pump.fun
func (d *pumpfunDEXAdapter) CalculateDiscretePnL(ctx context.Context, tokenAmount float64, initialInvestment float64) (*DiscreteTokenPnL, error) {
	if err := d.initPumpFun(ctx, d.tokenMint); err != nil {
		return nil, fmt.Errorf("failed to initialize Pump.fun: %w", err)
	}

	// Вызываем внутренний метод из пакета pumpfun
	result, err := d.inner.CalculateDiscretePnL(ctx, tokenAmount, initialInvestment)
	if err != nil {
		return nil, err
	}

	// Конвертируем тип pumpfun.DiscreteTokenPnL в dex.DiscreteTokenPnL
	return &DiscreteTokenPnL{
		CurrentPrice:      result.CurrentPrice,
		TheoreticalValue:  result.TheoreticalValue,
		SellEstimate:      result.SellEstimate,
		InitialInvestment: result.InitialInvestment,
		NetPnL:            result.NetPnL,
		PnLPercentage:     result.PnLPercentage,
	}, nil
}