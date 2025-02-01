// cmd/bot/main.go
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc"
	"github.com/rovshanmuradov/solana-bot/internal/config"
	"github.com/rovshanmuradov/solana-bot/internal/sniping"
	"github.com/rovshanmuradov/solana-bot/internal/storage/postgres"
	"github.com/rovshanmuradov/solana-bot/internal/types"
	"github.com/rovshanmuradov/solana-bot/internal/wallet"
	"go.uber.org/zap"
)

func main() {
	fmt.Println("\n=== Starting Solana Bot ===")

	// Создаем корневой контекст с отменой
	rootCtx, rootCancel := context.WithCancel(context.Background())
	defer rootCancel()

	// Создаем канал для ошибок снайпера
	sniperErrCh := make(chan error, 1)

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
	defer func() {
		if err := logger.Sync(); err != nil {
			fmt.Printf("Failed to sync logger: %v\n", err)
		}
	}()

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
		logger.Fatal("Failed to load wallets", zap.Error(err))
	}
	fmt.Printf("Loaded %d wallets\n", len(wallets))

	// Получаем первый кошелек для инициализации клиента
	var primaryWallet *wallet.Wallet
	for _, w := range wallets {
		primaryWallet = w
		break
	}
	if primaryWallet == nil {
		logger.Fatal("No wallets available")
	}
	// Инициализация Solana клиента с приватным ключом
	fmt.Println("=== Initializing Solana client ===")
	client, err := solbc.NewClient(cfg.RPCList, primaryWallet.PrivateKey, logger)
	if err != nil {
		logger.Fatal("Failed to initialize Solana client", zap.Error(err))
	}
	defer client.Close()

	// Инициализация блокчейнов
	fmt.Println("=== Initializing blockchains ===")
	blockchains := make(map[string]types.Blockchain)
	solanaBC, err := solbc.NewBlockchain(client, logger)
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

	// Создаем отдельный контекст для снайпера с таймаутом
	sniperCtx, sniperCancel := context.WithTimeout(rootCtx, 24*time.Hour) // или другой подходящий таймаут
	defer sniperCancel()

	// Запуск снайпера с обработкой ошибок
	fmt.Println("=== Running sniper ===")
	go func() {
		if err := sniper.Run(sniperCtx, tasks); err != nil {
			logger.Error("Sniper encountered an error", zap.Error(err))
			sniperErrCh <- err
			rootCancel() // Отменяем корневой контекст при ошибке
		}
	}()

	// Обработка сигналов завершения
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Ожидаем любое из событий
	select {
	case <-sigCh:
		logger.Info("Received termination signal")
	case <-rootCtx.Done():
		logger.Info("Context canceled")
	case err := <-sniperErrCh:
		logger.Error("Sniper failed", zap.Error(err))
	case <-sniperCtx.Done():
		if err := sniperCtx.Err(); err == context.DeadlineExceeded {
			logger.Error("Sniper timeout exceeded")
		}
	}

	// Начинаем корректное завершение работы
	logger.Info("Starting graceful shutdown")

	// Отменяем все контексты
	sniperCancel()
	rootCancel()

	// Даем время на завершение всех операций
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// Ждем завершения всех операций или таймаута
	select {
	case <-shutdownCtx.Done():
		logger.Warn("Shutdown timeout exceeded, forcing exit")
	case <-time.After(100 * time.Millisecond): // Минимальная пауза для корректного завершения
	}

	logger.Info("Bot successfully shut down")
}
