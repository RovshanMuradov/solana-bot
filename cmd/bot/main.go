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
	"github.com/rovshanmuradov/solana-bot/internal/types"
	"github.com/rovshanmuradov/solana-bot/internal/wallet"
	"go.uber.org/zap"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logger, err := zap.NewProduction()
	if err != nil {
		panic("Не удалось инициализировать логгер: " + err.Error())
	}
	defer func() {
		if err := logger.Sync(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to sync logger: %v\n", err)
		}
	}()

	cfg, err := config.LoadConfig("configs/config.json")
	if err != nil {
		logger.Fatal("Ошибка загрузки конфигурации", zap.Error(err))
	}

	wallets, err := wallet.LoadWallets("configs/wallets.csv")
	if err != nil {
		logger.Fatal("Ошибка загрузки кошельков", zap.Error(err))
	}

	client, err := solanaClient.NewClient(cfg.RPCList, logger)
	if err != nil {
		logger.Fatal("Ошибка инициализации клиента Solana", zap.Error(err))
	}

	blockchains := make(map[string]types.Blockchain)
	solanaBC, err := solanaClient.NewBlockchain(client, logger)
	if err != nil {
		logger.Fatal("Ошибка инициализации Solana blockchain", zap.Error(err))
	}
	blockchains["Solana"] = solanaBC

	tasks, err := sniping.LoadTasks("configs/tasks.csv")
	if err != nil {
		logger.Fatal("Ошибка загрузки задач", zap.Error(err))
	}

	sniper := sniping.NewSniper(blockchains, wallets, cfg, logger, client)

	go func() {
		sniper.Run(ctx, tasks)
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-sigCh:
		logger.Info("Получен сигнал завершения")
	case <-ctx.Done():
		logger.Info("Контекст отменен")
	}

	logger.Info("Начало graceful shutdown")
	cancel()
	logger.Info("Бот успешно завершил работу")
}
