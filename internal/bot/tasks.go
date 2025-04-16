// internal/bot/tasks.go
package bot

import (
	"context"
	"time"

	"github.com/rovshanmuradov/solana-bot/internal/monitor"
	"go.uber.org/zap"

	"github.com/rovshanmuradov/solana-bot/internal/dex"
	"github.com/rovshanmuradov/solana-bot/internal/task"
)

// handleSnipeTask выполняет операцию "снайпинга" (быстрой покупки) токенов с последующим
// мониторингом цены для автоматического выхода из позиции.
func (r *Runner) handleSnipeTask(ctx context.Context, t *task.Task, dexAdapter dex.DEX, dexTask *dex.Task, logger *zap.Logger) {
	// 1. Логируем начало операции
	logger.Info("Starting snipe operation",
		zap.String("task", t.TaskName),
		zap.String("token", t.TokenMint),
		zap.Float64("amount_sol", dexTask.AmountSol),
		zap.String("dex", dexAdapter.GetName()))

	// 2. Создаем контекст с таймаутом для выполнения операции
	opCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	// 3. Выполняем операцию снайпинга (быстрой покупки токена)
	err := dexAdapter.Execute(opCtx, dexTask)
	if err != nil {
		logger.Error("Snipe operation failed",
			zap.String("task", t.TaskName),
			zap.Error(err))
		return
	}

	logger.Info("Snipe operation completed successfully",
		zap.String("task", t.TaskName))

	// 4. Увеличенная задержка для обработки транзакции и создания токен-аккаунта
	time.Sleep(5 * time.Second)

	// 5. Получаем начальную цену токена
	initialPrice, err := dexAdapter.GetTokenPrice(ctx, t.TokenMint)
	if err != nil {
		logger.Warn("Failed to get initial token price, using estimated value",
			zap.String("task", t.TaskName),
			zap.Error(err))
		// В случае ошибки используем оценку цены на основе соотношения SOL/токены
		initialPrice = dexTask.AmountSol / 1000 // Предполагаем, что куплено ~1000 токенов
	}

	// 6. Получаем баланс приобретенных токенов с повторными попытками
	var tokenBalance uint64
	var tokenBalanceErr error

	// Настраиваем механизм retry
	maxRetries := 5
	retryDelay := time.Second

	for i := 0; i < maxRetries; i++ {
		tokenBalance, tokenBalanceErr = dexAdapter.GetTokenBalance(ctx, t.TokenMint)

		if tokenBalanceErr == nil && tokenBalance > 0 {
			// Баланс успешно получен, прерываем цикл повторных попыток
			break
		}

		// Если это последняя попытка, логируем ошибку и переходим к следующему шагу
		if i == maxRetries-1 {
			if tokenBalanceErr != nil {
				logger.Error("Failed to get token balance after multiple attempts",
					zap.String("task", t.TaskName),
					zap.Error(tokenBalanceErr))
			} else {
				logger.Warn("Token balance is zero after purchase",
					zap.String("task", t.TaskName))
			}
		} else {
			// Логируем попытку и ждем перед следующей
			if tokenBalanceErr != nil {
				logger.Debug("Failed to get token balance, retrying...",
					zap.String("task", t.TaskName),
					zap.Error(tokenBalanceErr),
					zap.Int("attempt", i+1),
					zap.Int("max_attempts", maxRetries))
			} else {
				logger.Debug("Token balance is zero, retrying...",
					zap.String("task", t.TaskName),
					zap.Int("attempt", i+1),
					zap.Int("max_attempts", maxRetries))
			}

			// Экспоненциальная задержка перед следующей попыткой
			time.Sleep(retryDelay)
			retryDelay *= 2 // Удваиваем задержку после каждой попытки
		}
	}

	// 7. Если не удалось получить баланс, пытаемся оценить его на основе затраченного SOL
	if tokenBalance == 0 {
		logger.Warn("Using estimated token balance based on SOL spent",
			zap.String("task", t.TaskName),
			zap.Float64("sol_spent", dexTask.AmountSol))

		// Оцениваем количество купленных токенов.
		// Используем предположение о покупке ~1000 токенов на 0.01 SOL при покупке нового токена
		estimatedTokens := 1000.0 * (dexTask.AmountSol / 0.01)
		tokenAmount := estimatedTokens
		// Конвертируем в минимальные единицы (обычно 6 десятичных знаков для токенов Solana)
		tokenBalance = uint64(estimatedTokens * 1e6)

		logger.Info("Using estimated token balance",
			zap.String("token", t.TokenMint),
			zap.Uint64("estimated_balance_raw", tokenBalance),
			zap.Float64("estimated_balance_human", tokenAmount))
	} else {
		// Конвертируем баланс токена из минимальных единиц в человекочитаемый формат
		tokenAmount := float64(tokenBalance) / 1e6

		logger.Info("Token purchased successfully",
			zap.String("token", t.TokenMint),
			zap.Uint64("balance_raw", tokenBalance),
			zap.Float64("balance_human", tokenAmount),
			zap.Float64("initial_price", initialPrice),
			zap.Float64("initial_investment", dexTask.AmountSol))
	}

	// Конвертируем баланс токена в человекочитаемый формат
	tokenAmount := float64(tokenBalance) / 1e6

	// 8. Создаем конфигурацию сессии мониторинга
	monitorConfig := &monitor.SessionConfig{
		TokenMint:       t.TokenMint,
		TokenAmount:     tokenAmount,
		TokenBalance:    tokenBalance,
		InitialAmount:   dexTask.AmountSol,
		InitialPrice:    initialPrice,
		MonitorInterval: dexTask.MonitorInterval,
		DEX:             dexAdapter,
		Logger:          logger.Named("monitor"),
		SlippagePercent: dexTask.SlippagePercent,
		PriorityFee:     dexTask.PriorityFee,
		ComputeUnits:    dexTask.ComputeUnits,
	}

	// 9. Создаем и запускаем сессию мониторинга
	session := monitor.NewMonitoringSession(monitorConfig)
	if err := session.Start(); err != nil {
		logger.Error("Failed to start monitoring session",
			zap.String("task", t.TaskName),
			zap.Error(err))
		return
	}

	logger.Info("Monitoring session started - press Enter to sell tokens or 'q' to exit",
		zap.String("task", t.TaskName))

	// 10. Ожидаем завершения мониторинга - этот вызов блокирует выполнение до
	// завершения мониторинга (например, когда пользователь нажмет Enter для продажи токенов)
	if err := session.Wait(); err != nil {
		logger.Error("Monitoring session error",
			zap.String("task", t.TaskName),
			zap.Error(err))
		return
	}

	logger.Info("Monitoring session completed",
		zap.String("task", t.TaskName))
}
