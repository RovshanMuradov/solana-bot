// cmd/bot/main.go
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	solanaClient "github.com/rovshanmuradov/solana-bot/internal/blockchain/solana"
	"github.com/rovshanmuradov/solana-bot/internal/config"
	"github.com/rovshanmuradov/solana-bot/internal/sniping"
	"github.com/rovshanmuradov/solana-bot/internal/storage/postgres"
	"github.com/rovshanmuradov/solana-bot/internal/types"
	"github.com/rovshanmuradov/solana-bot/internal/wallet"
	"go.uber.org/zap"
)

func main() {
	fmt.Println("\n=== Starting Solana Bot ===")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Инициализация логгера
	logConfig := zap.NewDevelopmentConfig()
	logConfig.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	logConfig.Development = true
	logConfig.DisableCaller = false
	logConfig.DisableStacktrace = false

	logger, err := logConfig.Build()
	if err != nil {
		fmt.Printf("Failed to initialize logger: %v\n", err)
		panic(err)
	}
	defer logger.Sync()

	fmt.Println("=== Loading configuration ===")
	// Загрузка конфигурации
	cfg, err := config.LoadConfig("configs/config.json")
	if err != nil {
		fmt.Printf("Failed to load configuration: %v\n", err)
		logger.Fatal("Failed to load configuration", zap.Error(err))
	}
	fmt.Printf("Config loaded: %+v\n", cfg)

	// Загрузка кошельков
	fmt.Println("=== Loading wallets ===")
	wallets, err := wallet.LoadWallets("configs/wallets.csv")
	if err != nil {
		fmt.Printf("Failed to load wallets: %v\n", err)
		logger.Fatal("Failed to load wallets", zap.Error(err))
	}
	fmt.Printf("Loaded %d wallets\n", len(wallets))

	// Инициализация Solana клиента
	fmt.Println("=== Initializing Solana client ===")
	client, err := solanaClient.NewClient(cfg.RPCList, logger)
	if err != nil {
		fmt.Printf("Failed to initialize Solana client: %v\n", err)
		logger.Fatal("Failed to initialize Solana client", zap.Error(err))
	}

	// Инициализация блокчейнов
	fmt.Println("=== Initializing blockchains ===")
	blockchains := make(map[string]types.Blockchain)
	solanaBC, err := solanaClient.NewBlockchain(client, logger)
	if err != nil {
		fmt.Printf("Failed to initialize Solana blockchain: %v\n", err)
		logger.Fatal("Failed to initialize Solana blockchain", zap.Error(err))
	}
	blockchains["Solana"] = solanaBC

	// Загрузка задач
	fmt.Println("=== Loading tasks ===")
	tasks, err := sniping.LoadTasks("configs/tasks.csv")
	if err != nil {
		fmt.Printf("Failed to load tasks: %v\n", err)
		logger.Fatal("Failed to load tasks", zap.Error(err))
	}
	for i, task := range tasks {
		fmt.Printf("Task %d: %+v\n", i+1, task)
	}

	// Инициализация хранилища
	fmt.Println("=== Initializing storage ===")
	storage, err := postgres.NewStorage(cfg.PostgresURL, logger)
	if err != nil {
		fmt.Printf("Failed to initialize storage: %v\n", err)
		logger.Fatal("Failed to initialize storage", zap.Error(err))
	}

	// Создание снайпера
	fmt.Println("=== Creating sniper ===")
	sniper := sniping.NewSniper(blockchains, wallets, cfg, logger, client, storage)
	if sniper == nil {
		fmt.Println("Failed to create sniper: sniper is nil")
		logger.Fatal("Failed to create sniper: sniper is nil")
	}

	// Запуск снайпера
	fmt.Println("=== Running sniper ===")
	go func() {
		sniper.Run(ctx, tasks)
	}()

	// Обработка сигналов завершения
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-sigCh:
		logger.Info("Received termination signal")
	case <-ctx.Done():
		logger.Info("Context canceled")
	}

	logger.Info("Starting graceful shutdown")
	cancel()
	logger.Info("Bot successfully shut down")
}
