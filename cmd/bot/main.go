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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Инициализация логгера
	logger, err := zap.NewProduction()
	if err != nil {
		panic("Failed to initialize logger: " + err.Error())
	}
	defer func() {
		if err := logger.Sync(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to sync logger: %v\n", err)
		}
	}()

	// Загрузка конфигурации
	cfg, err := config.LoadConfig("configs/config.json")
	if err != nil {
		logger.Fatal("Failed to load configuration", zap.Error(err))
	}

	// Инициализация хранилища и запуск миграций
	storage, err := postgres.NewStorage(cfg.PostgresURL, logger)
	if err != nil {
		logger.Fatal("Failed to initialize storage", zap.Error(err))
	}

	if err := storage.RunMigrations(""); err != nil {
		logger.Fatal("Failed to run migrations", zap.Error(err))
	}

	// Загрузка кошельков
	wallets, err := wallet.LoadWallets("configs/wallets.csv")
	if err != nil {
		logger.Fatal("Failed to load wallets", zap.Error(err))
	}

	// Инициализация Solana клиента
	client, err := solanaClient.NewClient(cfg.RPCList, logger)
	if err != nil {
		logger.Fatal("Failed to initialize Solana client", zap.Error(err))
	}

	// Инициализация блокчейнов
	blockchains := make(map[string]types.Blockchain)
	solanaBC, err := solanaClient.NewBlockchain(client, logger)
	if err != nil {
		logger.Fatal("Failed to initialize Solana blockchain", zap.Error(err))
	}
	blockchains["Solana"] = solanaBC

	// Загрузка задач
	tasks, err := sniping.LoadTasks("configs/tasks.csv")
	if err != nil {
		logger.Fatal("Failed to load tasks", zap.Error(err))
	}

	// Создание и запуск снайпера
	sniper := sniping.NewSniper(blockchains, wallets, cfg, logger, client, storage)

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
