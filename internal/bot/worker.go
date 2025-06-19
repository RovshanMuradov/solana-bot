// internal/bot/worker.go
package bot

import (
	"context"
	"fmt"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain"
	"sync"
	"time"

	"github.com/rovshanmuradov/solana-bot/internal/dex"
	"github.com/rovshanmuradov/solana-bot/internal/monitor"
	"github.com/rovshanmuradov/solana-bot/internal/task"
	"go.uber.org/zap"
)

type WorkerPool struct {
	wg           sync.WaitGroup
	ctx          context.Context
	tasks        <-chan *task.Task
	logger       *zap.Logger
	config       *task.Config
	solClient    *blockchain.Client
	wallets      map[string]*task.Wallet
	tradeHistory *monitor.TradeHistory // Phase 2: Trade logging
}

func NewWorkerPool(
	ctx context.Context,
	cfg *task.Config,
	logger *zap.Logger,
	solClient *blockchain.Client,
	wallets map[string]*task.Wallet,
	tasks <-chan *task.Task,
) *WorkerPool {
	// Phase 2: Initialize TradeHistory with 1000 entries in memory
	tradeHistory, err := monitor.NewTradeHistory("./logs", 1000, logger)
	if err != nil {
		logger.Error("Failed to create trade history, continuing without it", zap.Error(err))
		tradeHistory = nil
	} else {
		logger.Info("TradeHistory initialized",
			zap.String("log_dir", "./logs"),
			zap.Int("max_memory_trades", 1000))
	}

	return &WorkerPool{
		ctx:          ctx,
		config:       cfg,
		logger:       logger,
		tasks:        tasks,
		solClient:    solClient,
		wallets:      wallets,
		tradeHistory: tradeHistory,
	}
}

func (wp *WorkerPool) Start(n int) {
	for i := 0; i < n; i++ {
		wp.wg.Add(1)
		go wp.worker(i + 1)
	}
}

func (wp *WorkerPool) Wait() {
	wp.wg.Wait()
}

// Close closes the WorkerPool and its resources
func (wp *WorkerPool) Close() error {
	if wp.tradeHistory != nil {
		return wp.tradeHistory.Close()
	}
	return nil
}

func (wp *WorkerPool) worker(id int) {
	logger := wp.logger.Named(fmt.Sprintf("worker-%d", id))
	logger.Info("🚀 Trading worker started")

	for {
		select {
		case <-wp.ctx.Done():
			logger.Info("🛑 Worker shutting down due to context cancellation")
			return
		case t, ok := <-wp.tasks:
			if !ok {
				logger.Info("✅ All tasks completed")
				return
			}
			wp.handleTask(wp.ctx, t, logger)
		}
	}
}

func (wp *WorkerPool) handleTask(ctx context.Context, t *task.Task, logger *zap.Logger) {
	w := wp.wallets[t.WalletName]
	if w == nil {
		logger.Warn("⚠️  Skipping task - no wallet found: " + t.WalletName)
		return
	}

	dexAdapter, err := dex.GetDEXByName(t.Module, wp.solClient, w, logger)
	if err != nil {
		logger.Error(fmt.Sprintf("❌ DEX adapter init error for task '%s': %v", t.TaskName, err))
		return
	}

	logger.Info(fmt.Sprintf("⚡ Executing %s on %s for %s...%s",
		string(t.Operation),
		dexAdapter.GetName(),
		t.TokenMint[:4],
		t.TokenMint[len(t.TokenMint)-4:]))

	if t.Operation == task.OperationSnipe || t.Operation == task.OperationSwap {
		err := wp.handleMonitoredTask(ctx, t, dexAdapter, logger)
		if err != nil {
			logger.Error("❌ Monitored task failed: " + err.Error())
		}
	} else {
		err := dexAdapter.Execute(ctx, t)
		if err != nil {
			logger.Error(fmt.Sprintf("❌ Task execution failed for '%s': %v", t.TaskName, err))
		} else {
			logger.Info("🎉 Trade completed successfully: " + t.TaskName)
			// Phase 2: Log successful trade
			wp.logTrade(t, w, "", true, err)
		}
	}
}

func (wp *WorkerPool) handleMonitoredTask(ctx context.Context, t *task.Task, dexAdapter dex.DEX, logger *zap.Logger) error {
	logger.Info(fmt.Sprintf("📊 Starting monitored trade for %s...%s", t.TokenMint[:4], t.TokenMint[len(t.TokenMint)-4:]))

	if err := dexAdapter.Execute(ctx, t); err != nil {
		return fmt.Errorf("execute task: %w", err)
	}

	logger.Info("🎉 Trade executed successfully: " + t.TaskName)

	// Phase 2: Log successful monitored trade
	wallet := wp.wallets[t.WalletName]
	wp.logTrade(t, wallet, "", true, nil)

	var tokenBalance uint64
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Метка для выхода из цикла по условию
tokenLoop:
	for i := 0; i < 10; i++ {
		bal, err := dexAdapter.GetTokenBalance(checkCtx, t.TokenMint)
		if err != nil {
			logger.Warn(fmt.Sprintf("⚠️  GetTokenBalance failed (try %d): %v", i+1, err))
		} else if bal > 0 {
			tokenBalance = bal
			logger.Info(fmt.Sprintf("💰 Tokens received: %d", tokenBalance))
			break tokenLoop // сразу выходим из цикла tokenLoop
		}

		select {
		case <-checkCtx.Done():
			logger.Warn("⏰ Timeout waiting for token: " + t.TokenMint)
			break tokenLoop // тут тоже выходим из внешнего цикла
		case <-time.After(500 * time.Millisecond):
			// ждем и идем на следующую итерацию
		}
	}

	if tokenBalance == 0 {
		logger.Warn("⚠️  No tokens received; skipping monitor")
		return nil
	}

	// Создаем SellFunc для продажи токенов
	sellFn := CreateSellFunc(
		dexAdapter,
		t.TokenMint,
		t.SlippagePercent,
		t.PriorityFeeSol,
		t.ComputeUnits,
		logger.Named("sell"),
	)

	// Создаем и запускаем рабочий процесс мониторинга
	worker := NewMonitorWorker(
		ctx,
		t,
		dexAdapter,
		logger,
		tokenBalance,
		0, // Initial price will be fetched by monitor
		wp.config.MonitorDelay,
		sellFn,
	)

	// Запускаем и ожидаем завершения рабочего процесса
	err := worker.Start()
	if err != nil {
		logger.Error("❌ Monitor worker failed: " + err.Error())
		return err
	}

	return nil
}

// logTrade logs a trade to the TradeHistory
func (wp *WorkerPool) logTrade(t *task.Task, wallet *task.Wallet, txSignature string, success bool, err error) {
	if wp.tradeHistory == nil {
		return // TradeHistory not available
	}

	// Determine action based on task operation
	action := string(t.Operation)
	if action == "snipe" || action == "swap" {
		action = "buy"
	}

	// Create trade record
	trade := monitor.Trade{
		ID:          fmt.Sprintf("%s_%d_%d", action, t.ID, time.Now().Unix()),
		Timestamp:   time.Now(),
		WalletAddr:  wallet.PublicKey.String(),
		TokenMint:   t.TokenMint,
		TokenSymbol: extractTokenSymbol(t.TokenMint), // Extract symbol from mint
		Action:      action,
		AmountSOL:   t.AmountSol, // Correct field name
		AmountToken: 0,           // Will be updated later when balance is known
		Price:       0,           // Will be calculated from amount/tokens
		TxSignature: txSignature,
		DEX:         t.Module,
		Success:     success,
	}

	// Add error message if trade failed
	if err != nil {
		trade.ErrorMsg = err.Error()
	}

	// Log the trade
	if logErr := wp.tradeHistory.LogTrade(trade); logErr != nil {
		wp.logger.Error("Failed to log trade to history",
			zap.String("trade_id", trade.ID),
			zap.String("token", t.TokenMint),
			zap.Error(logErr))
	}

	// Phase 3: Check trade for volume alerts (if AlertManager available)
	// Note: AlertManager integration would be passed through config or dependency injection
	// For now, this is a placeholder for future AlertManager integration in WorkerPool
}

// extractTokenSymbol creates a short symbol from token mint address
func extractTokenSymbol(tokenMint string) string {
	if len(tokenMint) >= 8 {
		return tokenMint[:4] + "..." + tokenMint[len(tokenMint)-4:]
	}
	return "TOKEN"
}
