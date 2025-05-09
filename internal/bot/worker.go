// internal/bot/worker.go
package bot

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc"
	"github.com/rovshanmuradov/solana-bot/internal/dex"
	"github.com/rovshanmuradov/solana-bot/internal/monitor"
	"github.com/rovshanmuradov/solana-bot/internal/task"
	"github.com/rovshanmuradov/solana-bot/internal/wallet"
	"go.uber.org/zap"
)

type WorkerPool struct {
	wg        sync.WaitGroup
	ctx       context.Context
	tasks     <-chan *task.Task
	logger    *zap.Logger
	config    *task.Config
	solClient *solbc.Client
	wallets   map[string]*wallet.Wallet
}

func NewWorkerPool(
	ctx context.Context,
	cfg *task.Config,
	logger *zap.Logger,
	solClient *solbc.Client,
	wallets map[string]*wallet.Wallet,
	tasks <-chan *task.Task,
) *WorkerPool {
	return &WorkerPool{
		ctx:       ctx,
		config:    cfg,
		logger:    logger,
		tasks:     tasks,
		solClient: solClient,
		wallets:   wallets,
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

func (wp *WorkerPool) worker(id int) {
	logger := wp.logger.With(zap.Int("worker_id", id))
	logger.Info("Worker started")

	for {
		select {
		case <-wp.ctx.Done():
			logger.Info("Worker shutting down due to context cancellation")
			return
		case t, ok := <-wp.tasks:
			if !ok {
				logger.Info("Task channel closed")
				return
			}
			wp.handleTask(wp.ctx, t, logger)
		}
	}
}

func (wp *WorkerPool) handleTask(ctx context.Context, t *task.Task, logger *zap.Logger) {
	w := wp.wallets[t.WalletName]
	if w == nil {
		logger.Warn("Skipping task - no wallet found", zap.String("wallet", t.WalletName))
		return
	}

	dexAdapter, err := dex.GetDEXByName(t.Module, wp.solClient, w, logger)
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

	if t.Operation == task.OperationSnipe || t.Operation == task.OperationSwap {
		err := wp.handleMonitoredTask(ctx, t, dexAdapter, logger)
		if err != nil {
			logger.Error("Monitored task failed", zap.Error(err))
		}
	} else {
		err := dexAdapter.Execute(ctx, t)
		if err != nil {
			logger.Error("Task execution failed", zap.String("task", t.TaskName), zap.Error(err))
		} else {
			logger.Info("Task executed successfully", zap.String("task", t.TaskName))
		}
	}
}

func (wp *WorkerPool) handleMonitoredTask(ctx context.Context, t *task.Task, dexAdapter dex.DEX, logger *zap.Logger) error {
	logger.Info("Monitored task started", zap.String("token", t.TokenMint))

	if err := dexAdapter.Execute(ctx, t); err != nil {
		return fmt.Errorf("execute task: %w", err)
	}

	logger.Info("Operation completed successfully", zap.String("task", t.TaskName))

	var tokenBalance uint64
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	for i := 0; i < 10; i++ {
		bal, err := dexAdapter.GetTokenBalance(checkCtx, t.TokenMint)
		if err != nil {
			logger.Warn("GetTokenBalance failed", zap.Int("try", i+1), zap.Error(err))
		} else if bal > 0 {
			tokenBalance = bal
			logger.Info("Token received", zap.Uint64("balance", tokenBalance))
			break
		}
		select {
		case <-checkCtx.Done():
			logger.Warn("Timeout waiting for token", zap.String("token", t.TokenMint))
			break
		case <-time.After(500 * time.Millisecond):
		}
	}

	if tokenBalance == 0 {
		logger.Warn("No tokens received; skipping monitor")
		return nil
	}

	monitorConfig := &monitor.SessionConfig{
		Task:            t,
		TokenBalance:    tokenBalance,
		InitialPrice:    0,
		DEX:             dexAdapter,
		Logger:          logger.Named("monitor"),
		MonitorInterval: wp.config.MonitorDelay,
	}

	session := monitor.NewMonitoringSession(monitorConfig)

	if err := session.Start(); err != nil {
		logger.Error("Monitor session failed to start", zap.Error(err))
		return err
	}

	session.Wait()
	return nil
}
