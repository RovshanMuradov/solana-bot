// internal/bot/runner.go
package bot

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rovshanmuradov/solana-bot/internal/blockchain"
	"github.com/rovshanmuradov/solana-bot/internal/events"
	"github.com/rovshanmuradov/solana-bot/internal/task"
	"go.uber.org/zap"
)

type Runner struct {
	logger        *zap.Logger
	config        *task.Config
	solClient     *blockchain.Client
	taskManager   *task.Manager
	wallets       map[string]*task.Wallet
	defaultWallet *task.Wallet
	shutdownCh    chan os.Signal
	eventBus      *events.Bus
}

// NewRunner NewRunner: принимает cfg и logger
func NewRunner(cfg *task.Config, logger *zap.Logger) *Runner {
	// Загружаем кошельки
	wallets, err := task.LoadWallets("configs/wallets.yaml")
	if err != nil {
		logger.Fatal("Failed to load wallets", zap.Error(err))
	}

	var defaultW *task.Wallet
	for _, w := range wallets {
		defaultW = w
		break
	}

	// Create event bus
	eventBus := events.NewBus(logger, 1000)

	// Add debug logger for events
	eventBus.SubscribeFunc(events.OperationStarted, func(ctx context.Context, event events.Event) error {
		if e, ok := event.(*events.OperationStartedEvent); ok {
			logger.Debug("Operation started",
				zap.String("task", e.TaskName),
				zap.String("operation", e.Operation))
		}
		return nil
	})

	eventBus.SubscribeFunc(events.OperationCompleted, func(ctx context.Context, event events.Event) error {
		if e, ok := event.(*events.OperationCompletedEvent); ok {
			logger.Debug("Operation completed",
				zap.String("task", e.TaskName),
				zap.String("operation", e.Operation))
		}
		return nil
	})

	return &Runner{
		logger:        logger,
		config:        cfg,
		solClient:     blockchain.NewClient(cfg.RPCList[0], logger),
		taskManager:   task.NewManager(logger),
		wallets:       wallets,
		defaultWallet: defaultW,
		shutdownCh:    make(chan os.Signal, 1),
		eventBus:      eventBus,
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

	tasks, err := r.taskManager.LoadTasks("configs/tasks.yaml")
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
		r.eventBus,
	)

	workerPool.Start(numWorkers)
	workerPool.Wait()

	r.logger.Info("All workers finished")

	// Shutdown event bus
	shutdownTimeout := 5 * time.Second
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancel()
	if err := r.eventBus.Shutdown(shutdownCtx); err != nil {
		r.logger.Error("Failed to shutdown event bus", zap.Error(err))
	}

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
