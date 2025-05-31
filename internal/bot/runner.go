// internal/bot/runner.go
package bot

import (
	"context"
	"fmt"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain"
	"github.com/rovshanmuradov/solana-bot/internal/license"
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
		logger.Fatal("💥 Failed to load wallets: " + err.Error())
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
		r.logger.Info("📡 Signal received: " + sig.String())
		cancel()
	}()

	// Validate license first
	if err := r.validateLicense(ctx); err != nil {
		return fmt.Errorf("license validation failed: %w", err)
	}

	tasks, err := r.taskManager.LoadTasks("configs/tasks.csv")
	if err != nil {
		return err
	}
	r.logger.Info(fmt.Sprintf("📋 Loaded %d trading tasks", len(tasks)))

	taskCh := make(chan *task.Task, len(tasks))
	for _, t := range tasks {
		taskCh <- t
	}
	close(taskCh)

	numWorkers := r.config.Workers
	if numWorkers <= 0 {
		numWorkers = 1
	}
	r.logger.Info(fmt.Sprintf("🚀 Starting execution with %d workers", numWorkers))

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

	r.logger.Info("✅ All workers finished")
	return nil
}

func (r *Runner) Shutdown() {
	r.logger.Info("👋 Bot shutting down gracefully")

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
	r.logger.Info("📡 Signal received: " + sig.String())
	r.Shutdown()
}

// validateLicense validates the license using either Keygen or fallback validation
func (r *Runner) validateLicense(ctx context.Context) error {
	// Check if Keygen is configured
	if r.config.KeygenAccountID != "" && r.config.KeygenProductToken != "" && r.config.KeygenProductID != "" {
		return r.validateWithKeygen(ctx)
	}

	// Fallback to simple validation
	return r.validateSimple()
}

// validateWithKeygen validates license using Keygen.sh
func (r *Runner) validateWithKeygen(ctx context.Context) error {
	r.logger.Info("🔑 Validating license with Keygen.sh")

	// Use hardcoded Keygen credentials if not in config
	accountID := r.config.KeygenAccountID
	productToken := r.config.KeygenProductToken
	productID := r.config.KeygenProductID

	// Fallback to hardcoded values (for distribution)
	if accountID == "" {
		accountID = "c88da307-e118-4c8c-a8da-9cada169477b"
	}
	if productToken == "" {
		productToken = "prod-f716e07eabc338b13b7367e03074c33cb503562a92457ad6361c6a3060397fbdv3"
	}
	if productID == "" {
		productID = "60f40015-88e4-49e3-93a4-58303a91ee48"
	}

	validator := license.NewKeygenValidator(
		accountID,
		productToken,
		productID,
		r.logger,
	)

	if err := validator.ValidateLicense(ctx, r.config.License); err != nil {
		return fmt.Errorf("Keygen validation failed: %w", err)
	}

	r.logger.Info("✅ License validated with Keygen.sh")
	return nil
}

// validateSimple performs basic license validation (fallback)
func (r *Runner) validateSimple() error {
	if r.config.License == "" {
		return fmt.Errorf("license key is required")
	}

	if len(r.config.License) < 8 {
		return fmt.Errorf("license key is too short")
	}

	r.logger.Info("✅ License validated (basic mode)")
	return nil
}
