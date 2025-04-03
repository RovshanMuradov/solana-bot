// internal/bot/worker.go
package bot

import (
	"context"

	"go.uber.org/zap"

	"github.com/rovshanmuradov/solana-bot/internal/dex"
	"github.com/rovshanmuradov/solana-bot/internal/task"
)

// worker обрабатывает задачи из канала задач в отдельной горутине.
//
// Метод последовательно извлекает задачи из канала taskCh и обрабатывает их,
// пока канал не будет закрыт или контекст не будет отменен. Для каждой задачи
// вызывается метод processTask. Воркер идентифицируется уникальным числовым
// идентификатором, который используется в логах.
//
// Параметры:
//   - id: уникальный идентификатор воркера для отслеживания в логах
//   - ctx: контекст выполнения, используемый для отмены операций
//   - taskCh: канал, из которого воркер получает задачи для обработки
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
//
// Метод определяет кошелек для выполнения задачи, создает соответствующий
// DEX-адаптер и выполняет операцию в зависимости от типа задачи. Для операций
// типа OperationSnipe вызывается специализированный обработчик handleSnipeTask,
// который включает мониторинг цены после покупки. Для других типов операций
// (продажа, обмен) выполняется прямой вызов метода Execute адаптера DEX.
//
// Параметры:
//   - ctx: контекст выполнения для отмены операций
//   - t: информация о задаче, включая тип операции, параметры и целевой токен
//   - logger: настроенный логгер для записи информации о выполнении
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
	if t.Operation == task.OperationSnipe {
		r.handleSnipeTask(ctx, t, dexAdapter, dexTask, logger)
	} else {
		// Normal execution for sell/swap operations
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
