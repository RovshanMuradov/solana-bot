// internal/bot/runner.go
package bot

import (
	"context"
	"fmt"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain"
	"github.com/rovshanmuradov/solana-bot/internal/task"
	"go.uber.org/zap"
	"os"
	"os/signal"
	"syscall"
)

type Runner struct {
	logger        *zap.Logger
	config        *task.Config
	solClient     *blockchain.Client
	taskManager   *task.Manager
	wallets       map[string]*task.Wallet
	defaultWallet *task.Wallet
	shutdownCh    chan os.Signal
}

// NewRunner NewRunner: принимает cfg и logger
func NewRunner(cfg *task.Config, logger *zap.Logger) *Runner {
	// Загружаем кошельки
	wallets, err := task.LoadWallets("configs/wallets.csv")
	if err != nil {
		logger.Fatal("Failed to load wallets", zap.Error(err))
	}

	var defaultW *task.Wallet
	for _, w := range wallets {
		defaultW = w
		break
	}

	return &Runner{
		logger:        logger,
		config:        cfg,
		solClient:     blockchain.NewClient(cfg.RPCList[0], logger),
		taskManager:   task.NewManager(logger),
		wallets:       wallets,
		defaultWallet: defaultW,
		shutdownCh:    make(chan os.Signal, 1),
	}
}

func (r *Runner) Run(ctx context.Context) error {
	signal.Notify(r.shutdownCh, syscall.SIGINT, syscall.SIGTERM)
	shutdownCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		sig := <-r.shutdownCh
		r.logger.Info("Signal received", zap.String("signal", sig.String()))
		cancel()
	}()

	tasks, err := r.taskManager.LoadTasks("configs/tasks.csv")
	if err != nil {
		return err
	}
	r.logger.Info("Tasks loaded", zap.Int("count", len(tasks)))

	taskCh := make(chan *task.Task, len(tasks))
	for _, t := range tasks {
		taskCh <- t
	}
	close(taskCh)

	numWorkers := r.config.Workers
	if numWorkers <= 0 {
		numWorkers = 1
	}
	r.logger.Info("Starting task execution", zap.Int("workers", numWorkers))

	workerPool := NewWorkerPool(
		shutdownCtx,
		r.config,
		r.logger,
		r.solClient,
		r.wallets,
		taskCh,
	)

	workerPool.Start(numWorkers)
	workerPool.Wait()

	r.logger.Info("All workers finished")
	return nil
}

func (r *Runner) Shutdown() {
	r.logger.Info("Bot shutting down gracefully")

	if err := r.logger.Sync(); err != nil {
		if !os.IsNotExist(err) &&
			err.Error() != "sync /dev/stdout: invalid argument" &&
			err.Error() != "sync /dev/stderr: inappropriate ioctl for device" {
			fmt.Fprintf(os.Stderr, "failed to sync logger during shutdown: %v\n", err)
		}
	}
}

func (r *Runner) WaitForShutdown() {
	sig := <-r.shutdownCh
	r.logger.Info("Signal received", zap.String("signal", sig.String()))
	r.Shutdown()
}
