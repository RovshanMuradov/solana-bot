// ====================================
// File: cmd/bot/main.go (simplified)
// ====================================
package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"go.uber.org/zap"

	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc"
	"github.com/rovshanmuradov/solana-bot/internal/config"
	"github.com/rovshanmuradov/solana-bot/internal/dex"
	"github.com/rovshanmuradov/solana-bot/internal/storage/postgres"
	"github.com/rovshanmuradov/solana-bot/internal/utils/metrics"
	"github.com/rovshanmuradov/solana-bot/internal/wallet"
)

// Task represents the simplified CSV columns we care about.
type Task struct {
	TaskName            string
	Module              string
	WalletName          string
	Operation           string
	AmountSol           float64
	MinSolOutputSol     float64
	PriorityFeeLamports uint64
	ContractOrTokenMint string
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize logger
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()
	logger.Info("Starting simplified sniper bot")

	// Load config
	cfg, err := config.LoadConfig("configs/config.json")
	if err != nil {
		logger.Fatal("Failed to load config", zap.Error(err))
	}
	logger.Sugar().Infof("Config loaded: %+v", cfg)

	// Load wallets
	walletsMap, err := wallet.LoadWallets("configs/wallets.csv")
	if err != nil {
		logger.Fatal("Failed to load wallets", zap.Error(err))
	}
	logger.Info("Wallets loaded", zap.Int("count", len(walletsMap)))

	// Pick first wallet if not found in tasks
	var defaultWallet *wallet.Wallet
	for _, w := range walletsMap {
		defaultWallet = w
		break
	}

	// Init Solana client from first RPC
	solClient := solbc.NewClient(cfg.RPCList[0], logger)

	// Init metrics (Prometheus)
	metricsCollector := metrics.NewCollector()

	// Init Postgres
	store, err := postgres.NewStorage(cfg.PostgresURL, logger)
	if err != nil {
		logger.Fatal("Failed to init postgres", zap.Error(err))
	}
	if err := store.RunMigrations(); err != nil {
		logger.Fatal("Failed to run DB migrations", zap.Error(err))
	}
	logger.Info("Postgres ready")

	// Parse tasks.csv
	tasks, err := loadTasks("configs/tasks.csv")
	if err != nil {
		logger.Fatal("Failed to load tasks", zap.Error(err))
	}
	logger.Info("Tasks loaded", zap.Int("count", len(tasks)))

	// Execute tasks
	for _, t := range tasks {
		w := defaultWallet
		if walletsMap[t.WalletName] != nil {
			w = walletsMap[t.WalletName]
		}
		if w == nil {
			logger.Warn("Skipping task - no wallet found", zap.String("task", t.TaskName))
			continue
		}

		dexAdapter, err := dex.GetDEXByName(t.Module, solClient, w, logger, metricsCollector)
		if err != nil {
			logger.Error("DEX adapter init error", zap.String("task", t.TaskName), zap.Error(err))
			continue
		}

		logger.Info("Executing task",
			zap.String("task", t.TaskName),
			zap.String("operation", t.Operation),
			zap.String("DEX", dexAdapter.GetName()),
		)

		// Convert SOL amounts to lamports
		amountLamports := uint64(t.AmountSol * 1e9)
		minLamports := uint64(t.MinSolOutputSol * 1e9)

		taskData := &dex.Task{
			Operation:    dex.OperationType(t.Operation),
			Amount:       amountLamports,
			MinSolOutput: minLamports,
		}

		// Simple usage for pumpfun or raydium: we might set the "Mint" inside.
		// But in official code, you'd pass a separate param, or set it in the constructor.
		// For demonstration, we'll pretend the DEX logic picks up t.ContractOrTokenMint as needed.

		err = dexAdapter.Execute(ctx, taskData)
		if err != nil {
			logger.Error("Error executing snipe operation",
				zap.String("task", t.TaskName),
				zap.Error(err),
			)
		} else {
			logger.Info("Snipe operation completed",
				zap.String("task", t.TaskName))
		}
	}

	// Wait for signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	logger.Info("Bot started. Waiting for exit signal...")
	sig := <-sigCh
	logger.Info("Signal received", zap.String("signal", sig.String()))
	logger.Info("Bot shutting down gracefully")
}

// loadTasks reads a simple CSV with columns:
// task_name,module,wallet,operation,amount_sol,min_sol_output_sol,priority_fee_lamports,contract_address
func loadTasks(path string) ([]Task, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	records, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) < 2 {
		return nil, fmt.Errorf("no tasks found in CSV")
	}

	var tasks []Task
	// Skip header
	for _, row := range records[1:] {
		if len(row) < 8 {
			continue
		}
		amountSol, _ := strconv.ParseFloat(row[4], 64)
		minSolOut, _ := strconv.ParseFloat(row[5], 64)
		priority, _ := strconv.ParseUint(row[6], 10, 64)

		tasks = append(tasks, Task{
			TaskName:            row[0],
			Module:              row[1],
			WalletName:          row[2],
			Operation:           row[3],
			AmountSol:           amountSol,
			MinSolOutputSol:     minSolOut,
			PriorityFeeLamports: priority,
			ContractOrTokenMint: row[7],
		})
	}
	return tasks, nil
}
