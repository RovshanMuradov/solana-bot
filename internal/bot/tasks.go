// internal/bot/tasks.go
package bot

import (
	"context"
	"math"
	"time"

	"go.uber.org/zap"

	"github.com/rovshanmuradov/solana-bot/internal/dex"
	"github.com/rovshanmuradov/solana-bot/internal/monitor"
	"github.com/rovshanmuradov/solana-bot/internal/task"
)

// handleSnipeTask выполняет операцию "снайпинга" (быстрой покупки) токенов с последующим
// мониторингом цены для автоматического выхода из позиции.
//
// Метод сначала выполняет покупку токена через указанный DEX-адаптер. После успешной покупки
// получает начальную цену токена и текущий баланс, затем создает и запускает сессию мониторинга.
// Сессия мониторинга отслеживает изменение цены токена с заданным интервалом и может
// автоматически продать токены при достижении заданных условий (take-profit или stop-loss).
//
// Процесс выполнения:
//  1. Выполнение операции покупки токена (снайпинг)
//  2. Получение начальной цены и баланса токена
//  3. Создание сессии мониторинга с параметрами из исходной задачи
//  4. Запуск мониторинга и ожидание его завершения
//
// Параметры:
//   - ctx: контекст выполнения для отмены операций
//   - t: информация о задаче, включая название и параметры
//   - dexAdapter: интерфейс для взаимодействия с децентрализованной биржей
//   - dexTask: параметры задачи, специфичные для DEX-операций
//   - logger: настроенный логгер для записи информации о выполнении
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

	// Get actual token balance after purchase
	balanceCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	tokenBalance, err := dexAdapter.GetTokenBalance(balanceCtx, dexTask.TokenMint)
	cancel()

	if err != nil {
		logger.Error("Failed to get token balance",
			zap.String("task", t.TaskName),
			zap.Error(err))
		return
	}

	// Convert token balance to human-readable format
	// Assume 6 decimal places for tokens, can be adjusted based on token precision
	const tokenDecimals = 6
	tokenAmount := float64(tokenBalance) / math.Pow10(tokenDecimals)

	logger.Info("Token purchase details",
		zap.String("task", t.TaskName),
		zap.Float64("initial_price_sol", initialPrice),
		zap.Uint64("token_balance", tokenBalance),
		zap.Float64("token_amount", tokenAmount))

	// Create monitor session config with all transaction parameters from the original task
	monitorConfig := &monitor.SessionConfig{
		TokenMint:       dexTask.TokenMint,
		TokenAmount:     tokenAmount,
		TokenBalance:    tokenBalance,
		InitialAmount:   dexTask.AmountSol,
		InitialPrice:    initialPrice,
		MonitorInterval: r.config.PriceDelay,
		DEX:             dexAdapter,
		Logger:          logger.Named("monitor"),

		// Передаем параметры транзакции из оригинальной задачи
		SlippagePercent: dexTask.SlippagePercent,
		PriorityFee:     dexTask.PriorityFee,
		ComputeUnits:    dexTask.ComputeUnits,
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
