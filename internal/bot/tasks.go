// internal/bot/tasks.go
package bot

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/rovshanmuradov/solana-bot/internal/dex"
	"github.com/rovshanmuradov/solana-bot/internal/monitor"
	"github.com/rovshanmuradov/solana-bot/internal/task"
)

// handleSnipeTask processes a snipe operation with price monitoring
func (r *Runner) handleSnipeTask(ctx context.Context, t *task.Task, dexAdapter dex.DEX, dexTask *dex.Task, logger *zap.Logger) {
	logger.Info("Executing snipe operation with monitoring",
		zap.String("task", t.TaskName),
		zap.Duration("monitor_interval", dexTask.MonitorInterval))

	// Execute the snipe operation
	err := dexAdapter.Execute(ctx, dexTask)
	if err != nil {
		logger.Error("Error during snipe operation",
			zap.String("task", t.TaskName),
			zap.Error(err))
		return // Skip monitoring if snipe fails
	}

	logger.Info("Snipe completed, starting monitoring",
		zap.String("task", t.TaskName))

	// Get initial token price
	priceCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	initialPrice, err := dexAdapter.GetTokenPrice(priceCtx, dexTask.TokenMint)
	cancel()

	if err != nil {
		logger.Error("Failed to get initial token price",
			zap.String("task", t.TaskName),
			zap.Error(err))
		return
	}

	// Create monitor session config
	tokenAmount := dexTask.AmountSol * 10 // Пример, должно быть фактическое количество токенов
	monitorConfig := &monitor.SessionConfig{
		TokenMint:       dexTask.TokenMint,
		TokenAmount:     tokenAmount,
		InitialAmount:   dexTask.AmountSol,
		InitialPrice:    initialPrice,
		MonitorInterval: r.config.PriceDelay,
		DEX:             dexAdapter,
		Logger:          logger.Named("monitor"),
	}

	// Create and start monitoring session
	session := monitor.NewMonitoringSession(monitorConfig)
	if err := session.Start(); err != nil {
		logger.Error("Failed to start monitoring session",
			zap.String("task", t.TaskName),
			zap.Error(err))
		return
	}

	// Wait for session to complete
	if err := session.Wait(); err != nil {
		logger.Error("Error during monitoring session",
			zap.String("task", t.TaskName),
			zap.Error(err))
	} else {
		logger.Info("Monitoring session completed successfully",
			zap.String("task", t.TaskName))
	}
}
