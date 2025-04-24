// internal/bot/worker.go
package bot

import (
	"context"
	"github.com/rovshanmuradov/solana-bot/internal/monitor"
	"time"

	"go.uber.org/zap"

	"github.com/rovshanmuradov/solana-bot/internal/dex"
	"github.com/rovshanmuradov/solana-bot/internal/task"
)

// worker обрабатывает задачи из канала задач в отдельной горутине.
func (r *Runner) worker(id int, ctx context.Context, taskCh <-chan *task.Task) {
	logger := r.logger.With(zap.Int("worker_id", id))
	logger.Debug("Worker started")

	for t := range taskCh {
		// Check if context is cancelled
		select {
		case <-ctx.Done():
			logger.Debug("Worker shutting down due to context cancellation")
			return
		default:
			// Continue processing
		}

		logger.Info("Processing task",
			zap.String("task", t.TaskName),
			zap.String("operation", string(t.Operation)))

		// Process the task
		r.processTask(ctx, t, logger)
	}

	logger.Debug("Worker finished, no more tasks")
}

// processTask обрабатывает выполнение одной задачи.
func (r *Runner) processTask(ctx context.Context, t *task.Task, logger *zap.Logger) {
	// Get wallet for this task
	w := r.defaultWallet
	if r.wallets[t.WalletName] != nil {
		w = r.wallets[t.WalletName]
	}
	if w == nil {
		logger.Warn("Skipping task - no wallet found", zap.String("task", t.TaskName))
		return
	}

	// Get DEX adapter
	dexAdapter, err := dex.GetDEXByName(t.Module, r.solClient, w, logger)
	if err != nil {
		logger.Error("DEX adapter init error", zap.String("task", t.TaskName), zap.Error(err))
		return
	}

	logger.Info("Executing task",
		zap.String("task", t.TaskName),
		zap.String("operation", string(t.Operation)),
		zap.String("DEX", dexAdapter.GetName()),
		zap.String("token_mint", t.TokenMint),
	)

	// Create a time-based monitor interval from config
	monitorInterval := r.config.PriceDelay
	dexTask := t.ToDEXTask(monitorInterval)

	// Handle task based on operation type
	// Запускаем мониторинг как для операции Snipe, так и для операции Swap
	if t.Operation == task.OperationSnipe || t.Operation == task.OperationSwap {
		r.handleMonitoredTask(ctx, t, dexAdapter, dexTask, logger)
	} else {
		// Normal execution for другие операции
		err = dexAdapter.Execute(ctx, dexTask)
		if err != nil {
			logger.Error("Error executing operation",
				zap.String("task", t.TaskName),
				zap.Error(err),
			)
		} else {
			logger.Info("Operation completed",
				zap.String("task", t.TaskName))
		}
	}
}

// handleMonitoredTask выполняет операцию и запускает мониторинг цены токена.
// Это обобщенная версия функции handleSnipeTask, которая работает для разных типов операций.
func (r *Runner) handleMonitoredTask(ctx context.Context, t *task.Task, dexAdapter dex.DEX, dexTask *dex.Task, logger *zap.Logger) {
	logger.Info("Starting operation with monitoring",
		zap.String("task", t.TaskName),
		zap.String("token", t.TokenMint),
		zap.String("operation", string(t.Operation)),
		zap.Float64("amount_sol", dexTask.AmountSol),
		zap.String("dex", dexAdapter.GetName()))

	opCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	if err := dexAdapter.Execute(opCtx, dexTask); err != nil {
		logger.Error("Operation failed", zap.String("task", t.TaskName), zap.Error(err))
		return
	}
	logger.Info("Operation completed successfully", zap.String("task", t.TaskName))

	time.Sleep(5 * time.Second)

	monitorConfig := &monitor.SessionConfig{
		TokenMint:       t.TokenMint,
		InitialAmount:   dexTask.AmountSol,
		MonitorInterval: dexTask.MonitorInterval,
		DEX:             dexAdapter,
		Logger:          logger.Named("monitor"),
		SlippagePercent: dexTask.SlippagePercent,
		PriorityFee:     dexTask.PriorityFee,
		ComputeUnits:    dexTask.ComputeUnits,
	}

	session := monitor.NewMonitoringSession(monitorConfig)
	if err := session.Start(); err != nil {
		logger.Error("Failed to start monitoring session", zap.String("task", t.TaskName), zap.Error(err))
		return
	}

	logger.Info("Monitoring session started - press Enter to sell tokens or 'q' to exit", zap.String("task", t.TaskName))

	if err := session.Wait(); err != nil {
		logger.Error("Monitoring session error", zap.String("task", t.TaskName), zap.Error(err))
	}
}
