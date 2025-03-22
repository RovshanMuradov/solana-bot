// =============================================
// File: internal/bot/runner.go
// =============================================
package bot

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc"
	"github.com/rovshanmuradov/solana-bot/internal/dex"
	"github.com/rovshanmuradov/solana-bot/internal/monitor"
	"github.com/rovshanmuradov/solana-bot/internal/storage"
	"github.com/rovshanmuradov/solana-bot/internal/storage/postgres"
	"github.com/rovshanmuradov/solana-bot/internal/task"
	"github.com/rovshanmuradov/solana-bot/internal/utils/metrics"
	"github.com/rovshanmuradov/solana-bot/internal/wallet"
)

// Runner represents the main bot process controller
type Runner struct {
	logger           *zap.Logger
	config           *task.Config
	solClient        *solbc.Client
	metricsCollector *metrics.Collector
	db               storage.Storage // Changed from *postgres.Storage to storage.Storage
	taskManager      *task.Manager
	wallets          map[string]*wallet.Wallet
	defaultWallet    *wallet.Wallet
	shutdownCh       chan os.Signal
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

	// Initialize metrics collector
	r.metricsCollector = metrics.NewCollector()
	r.metricsCollector.SetSolanaClient(r.solClient)
	r.metricsCollector.SetDefaultWallet(r.defaultWallet)

	// Initialize task manager
	r.taskManager = task.NewManager(r.logger)

	// Initialize postgres
	store, err := postgres.NewStorage(cfg.PostgresURL, r.logger)
	if err != nil {
		return err
	}
	if err := store.RunMigrations(); err != nil {
		return err
	}
	r.db = store
	r.logger.Info("Postgres ready")

	return nil
}

// Run executes the main bot logic
func (r *Runner) Run(ctx context.Context) error {
	// Setup signal handling
	signal.Notify(r.shutdownCh, syscall.SIGINT, syscall.SIGTERM)

	// Start background monitoring for shutdown signal
	shutdownCtx, cancel := context.WithCancel(ctx)
	defer cancel()

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

	// Process each task
	for _, t := range tasks {
		// Check for shutdown
		select {
		case <-shutdownCtx.Done():
			r.logger.Info("Shutdown requested, stopping task processing")
			return nil
		default:
			// Continue processing
		}

		// Get wallet for this task
		w := r.defaultWallet
		if r.wallets[t.WalletName] != nil {
			w = r.wallets[t.WalletName]
		}
		if w == nil {
			r.logger.Warn("Skipping task - no wallet found", zap.String("task", t.TaskName))
			continue
		}

		// Get DEX adapter
		dexAdapter, err := dex.GetDEXByName(t.Module, r.solClient, w, r.logger, r.metricsCollector)
		if err != nil {
			r.logger.Error("DEX adapter init error", zap.String("task", t.TaskName), zap.Error(err))
			continue
		}

		r.logger.Info("Executing task",
			zap.String("task", t.TaskName),
			zap.String("operation", t.Operation),
			zap.String("DEX", dexAdapter.GetName()),
			zap.String("token_mint", t.ContractOrTokenMint),
		)

		// Convert task to DEX format and execute
		dexTask := t.ToDEXTask()

		// Execute operation
		if dexTask.Operation == dex.OperationSnipe {
			r.logger.Info("Executing snipe operation with monitoring",
				zap.String("task", t.TaskName),
				zap.Duration("monitor_interval", dexTask.MonitorInterval))

			// Execute the snipe operation
			err = dexAdapter.Execute(shutdownCtx, dexTask)
			if err != nil {
				r.logger.Error("Error during snipe operation",
					zap.String("task", t.TaskName),
					zap.Error(err))
				continue // Skip monitoring if snipe fails
			}

			r.logger.Info("Snipe completed, starting monitoring",
				zap.String("task", t.TaskName))

			// Get initial token price
			ctx, cancel := context.WithTimeout(shutdownCtx, 10*time.Second)
			initialPrice, err := dexAdapter.GetTokenPrice(ctx, dexTask.TokenMint)
			cancel()

			if err != nil {
				r.logger.Error("Failed to get initial token price",
					zap.String("task", t.TaskName),
					zap.Error(err))
				continue
			}

			// Create monitor session config
			// Assuming the tokensReceived is the AmountSol for now (this should be updated to actual tokens received)
			// In a real implementation, we would capture the actual tokens received from the snipe operation
			tokenAmount := dexTask.AmountSol * 10 // This is just an example, should be actual token amount

			// Convert priceDelay from milliseconds to time.Duration
			// price_delay is in milliseconds where 100-1000 = 1 second
			monitorInterval := time.Duration(r.config.PriceDelay) * time.Millisecond
			
			monitorConfig := &monitor.SessionConfig{
				TokenMint:       dexTask.TokenMint,
				TokenAmount:     tokenAmount,       // This should be the actual received tokens
				InitialAmount:   dexTask.AmountSol, // SOL spent
				InitialPrice:    initialPrice,      // Initial price
				MonitorInterval: monitorInterval,   // Use price_delay from config.json
				DEX:             dexAdapter,
				Logger:          r.logger.Named("monitor"),
			}

			// Create and start monitoring session
			session := monitor.NewMonitoringSession(monitorConfig)
			if err := session.Start(); err != nil {
				r.logger.Error("Failed to start monitoring session",
					zap.String("task", t.TaskName),
					zap.Error(err))
				continue
			}

			// Wait for session to complete (this blocks until user input or context cancellation)
			if err := session.Wait(); err != nil {
				r.logger.Error("Error during monitoring session",
					zap.String("task", t.TaskName),
					zap.Error(err))
			} else {
				r.logger.Info("Monitoring session completed successfully",
					zap.String("task", t.TaskName))
			}
		} else {
			// Normal execution for sell operations
			err = dexAdapter.Execute(shutdownCtx, dexTask)
			if err != nil {
				r.logger.Error("Error executing operation",
					zap.String("task", t.TaskName),
					zap.Error(err),
				)
			} else {
				r.logger.Info("Operation completed",
					zap.String("task", t.TaskName))
			}
		}
	}

	r.logger.Info("All tasks completed")
	return nil
}

// Shutdown performs graceful shutdown
func (r *Runner) Shutdown() {
	r.logger.Info("Bot shutting down gracefully")
	// Close resources and connections
	if r.db != nil {
		// If your storage implementation provides a Close method, it should be added to the interface
		if closer, ok := r.db.(interface{ Close() error }); ok {
			if err := closer.Close(); err != nil {
				r.logger.Error("Error closing database connection", zap.Error(err))
			}
		}
	}
}

// WaitForShutdown blocks until shutdown signal is received
func (r *Runner) WaitForShutdown() {
	sig := <-r.shutdownCh
	r.logger.Info("Signal received", zap.String("signal", sig.String()))
	r.Shutdown()
}
