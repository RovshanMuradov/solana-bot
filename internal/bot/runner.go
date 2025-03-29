// internal/bot/runner.go
package bot

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc"
	"github.com/rovshanmuradov/solana-bot/internal/task"
	"github.com/rovshanmuradov/solana-bot/internal/wallet"
)

// Runner represents the main bot process controller
type Runner struct {
	logger        *zap.Logger
	config        *task.Config
	solClient     *solbc.Client
	taskManager   *task.Manager
	wallets       map[string]*wallet.Wallet
	defaultWallet *wallet.Wallet
	shutdownCh    chan os.Signal
}

// NewRunner creates a new bot runner instance
func NewRunner(logger *zap.Logger) *Runner {
	return &Runner{
		logger:     logger,
		shutdownCh: make(chan os.Signal, 1),
	}
}

// Initialize sets up all dependencies
func (r *Runner) Initialize(configPath string) error {
	r.logger.Info("Initializing bot runner")

	// Load configuration
	cfg, err := task.LoadConfig(configPath)
	if err != nil {
		return err
	}
	r.config = cfg
	r.logger.Sugar().Infof("Config loaded: %+v", cfg)

	// Load wallets
	wallets, err := wallet.LoadWallets("configs/wallets.csv")
	if err != nil {
		return err
	}
	r.wallets = wallets
	r.logger.Info("Wallets loaded", zap.Int("count", len(wallets)))

	// Pick first wallet as default if needed
	for _, w := range wallets {
		r.defaultWallet = w
		break
	}

	// Initialize Solana client
	r.solClient = solbc.NewClient(cfg.RPCList[0], r.logger)

	// Initialize task manager
	r.taskManager = task.NewManager(r.logger)

	return nil
}

// Run executes the main bot logic with parallel workers
func (r *Runner) Run(ctx context.Context) error {
	// Setup signal handling
	signal.Notify(r.shutdownCh, syscall.SIGINT, syscall.SIGTERM)

	// Create a context that can be cancelled
	shutdownCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Setup handler for shutdown signal
	go func() {
		sig := <-r.shutdownCh
		r.logger.Info("Signal received", zap.String("signal", sig.String()))
		cancel()
	}()

	// Load task definitions
	tasks, err := r.taskManager.LoadTasks("configs/tasks.csv")
	if err != nil {
		return err
	}
	r.logger.Info("Tasks loaded", zap.Int("count", len(tasks)))

	// Create a task channel and add tasks to it
	taskCh := make(chan *task.Task, len(tasks))
	for _, t := range tasks {
		taskCh <- t
	}
	close(taskCh)

	// Determine number of workers from config
	numWorkers := r.config.Workers
	if numWorkers <= 0 {
		numWorkers = 1
	}
	r.logger.Info("Starting task execution", zap.Int("workers", numWorkers))

	// Create a wait group to track worker completion
	var wg sync.WaitGroup
	wg.Add(numWorkers)

	// Start workers
	for i := 0; i < numWorkers; i++ {
		workerID := i + 1
		go func(id int) {
			defer wg.Done()
			r.worker(id, shutdownCtx, taskCh)
		}(workerID)
	}

	// Wait for workers to complete or context to be cancelled
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	// Wait for workers to finish or context to be cancelled
	select {
	case <-done:
		r.logger.Info("All tasks completed successfully")
	case <-shutdownCtx.Done():
		r.logger.Info("Execution interrupted, waiting for workers to finish")
		// Wait for workers to finish gracefully
		select {
		case <-done:
			r.logger.Info("All workers finished gracefully")
		case <-time.After(5 * time.Second):
			r.logger.Warn("Not all workers finished in time")
		}
	}

	return nil
}

// Shutdown performs graceful shutdown
func (r *Runner) Shutdown() {
	r.logger.Info("Bot shutting down gracefully")
	// Здесь может быть код для корректного завершения всех подсистем
}

// WaitForShutdown blocks until shutdown signal is received
func (r *Runner) WaitForShutdown() {
	sig := <-r.shutdownCh
	r.logger.Info("Signal received", zap.String("signal", sig.String()))
	r.Shutdown()
}
