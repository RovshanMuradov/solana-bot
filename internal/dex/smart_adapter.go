// =============================================
// File: internal/dex/smart_adapter.go
// =============================================
package dex

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/rovshanmuradov/solana-bot/internal/dex/model"
	"github.com/rovshanmuradov/solana-bot/internal/task"
)

// smartDEXAdapter автоматически определяет, какой DEX использовать (pumpfun или pumpswap)
// в зависимости от состояния токена.
type smartDEXAdapter struct {
	baseDEXAdapter
	// Адаптеры для обоих DEX
	pumpfunAdapter  *pumpfunDEXAdapter
	pumpswapAdapter *pumpswapDEXAdapter
	// Кеш для оптимизации
	determinedDEX DEX
	cacheMu       sync.Mutex
	cacheData     struct {
		lastUpdate time.Time
		tokenMint  string
	}
}

// Execute выполняет операцию, автоматически определяя, какой DEX использовать.
func (d *smartDEXAdapter) Execute(ctx context.Context, t *task.Task) error {
	if t.TokenMint == "" {
		return fmt.Errorf("token mint is required")
	}

	// Определяем, какой DEX использовать
	dex, err := d.determineDEX(ctx, t.TokenMint)
	if err != nil {
		return fmt.Errorf("failed to determine DEX: %w", err)
	}

	// Создаем копию таска с адаптированной операцией
	adaptedTask := *t
	if dex == d.pumpfunAdapter {
		// Для pumpfun используем операцию OperationSnipe
		adaptedTask.Operation = task.OperationSnipe
		d.logger.Info("Using Pump.fun for snipe operation", zap.String("token_mint", t.TokenMint))
	} else {
		// Для pumpswap используем операцию OperationSwap
		adaptedTask.Operation = task.OperationSwap
		d.logger.Info("Using Pump.swap for snipe operation", zap.String("token_mint", t.TokenMint))
	}

	// Выполняем операцию на выбранном DEX
	return dex.Execute(ctx, &adaptedTask)
}

// determineDEX определяет, какой DEX использовать для данного токена.
func (d *smartDEXAdapter) determineDEX(ctx context.Context, tokenMint string) (DEX, error) {
	d.cacheMu.Lock()
	defer d.cacheMu.Unlock()

	// Проверяем кеш
	if d.determinedDEX != nil &&
		d.cacheData.tokenMint == tokenMint &&
		time.Since(d.cacheData.lastUpdate) < 30*time.Second {
		return d.determinedDEX, nil
	}

	// Гарантируем, что оба адаптера инициализированы
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

	// Пытаемся сначала определить, можно ли использовать pumpfun
	d.logger.Info("Checking if token can be handled by Pump.fun", zap.String("token", tokenMint))

	// Инициализируем pumpfun адаптер
	err := d.pumpfunAdapter.init(ctx, tokenMint, d.pumpfunAdapter.makeInitPumpFun(tokenMint))
	if err != nil {
		d.logger.Warn("Failed to initialize Pump.fun adapter, falling back to Pump.swap",
			zap.String("token", tokenMint),
			zap.Error(err))

		// В случае ошибки используем pumpswap
		d.determinedDEX = d.pumpswapAdapter
		d.cacheData.tokenMint = tokenMint
		d.cacheData.lastUpdate = time.Now()
		return d.pumpswapAdapter, nil
	}

	// Проверяем состояние bonding curve
	if d.pumpfunAdapter.inner == nil {
		d.logger.Warn("Pump.fun inner DEX is nil, falling back to Pump.swap",
			zap.String("token", tokenMint))
		d.determinedDEX = d.pumpswapAdapter
	} else {
		isComplete, err := d.pumpfunAdapter.inner.IsBondingCurveComplete(ctx)
		if err != nil {
			d.logger.Warn("Failed to check bonding curve status, falling back to Pump.swap",
				zap.String("token", tokenMint),
				zap.Error(err))
			d.determinedDEX = d.pumpswapAdapter
		} else if isComplete {
			// Если bonding curve complete, используем pumpswap
			d.logger.Info("Bonding curve is complete, using Pump.swap",
				zap.String("token", tokenMint))
			d.determinedDEX = d.pumpswapAdapter
		} else {
			// Если bonding curve не complete, используем pumpfun
			d.logger.Info("Bonding curve is not complete, using Pump.fun",
				zap.String("token", tokenMint))
			d.determinedDEX = d.pumpfunAdapter
		}
	}

	d.cacheData.tokenMint = tokenMint
	d.cacheData.lastUpdate = time.Now()
	return d.determinedDEX, nil
}

// GetTokenPrice получает цену токена, автоматически выбирая подходящий DEX.
func (d *smartDEXAdapter) GetTokenPrice(ctx context.Context, tokenMint string) (float64, error) {
	dex, err := d.determineDEX(ctx, tokenMint)
	if err != nil {
		return 0, fmt.Errorf("determine DEX: %w", err)
	}
	return dex.GetTokenPrice(ctx, tokenMint)
}

// GetTokenBalance получает баланс токена, автоматически выбирая подходящий DEX.
func (d *smartDEXAdapter) GetTokenBalance(ctx context.Context, tokenMint string) (uint64, error) {
	dex, err := d.determineDEX(ctx, tokenMint)
	if err != nil {
		return 0, fmt.Errorf("determine DEX: %w", err)
	}
	return dex.GetTokenBalance(ctx, tokenMint)
}

// SellPercentTokens продает процент токенов, автоматически выбирая подходящий DEX.
func (d *smartDEXAdapter) SellPercentTokens(ctx context.Context, tokenMint string, pct, slip float64, fee string, cu uint32) error {
	dex, err := d.determineDEX(ctx, tokenMint)
	if err != nil {
		return fmt.Errorf("determine DEX: %w", err)
	}
	return dex.SellPercentTokens(ctx, tokenMint, pct, slip, fee, cu)
}

// CalculatePnL рассчитывает PnL, автоматически выбирая подходящий DEX.
func (d *smartDEXAdapter) CalculatePnL(ctx context.Context, amount, invest float64) (*model.PnLResult, error) {
	d.mu.Lock()
	tokenMint := d.tokenMint
	d.mu.Unlock()

	dex, err := d.determineDEX(ctx, tokenMint)
	if err != nil {
		return nil, fmt.Errorf("determine DEX: %w", err)
	}
	return dex.CalculatePnL(ctx, amount, invest)
}
