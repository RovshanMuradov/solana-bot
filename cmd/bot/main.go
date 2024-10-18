package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/rovshanmuradov/solana-bot/internal/config"
	"github.com/rovshanmuradov/solana-bot/internal/sniping"
	"github.com/rovshanmuradov/solana-bot/internal/wallet"
	solanaclient "github.com/rovshanmuradov/solana-bot/pkg/blockchain/solana"
	"go.uber.org/zap"
)

func main() {
	// Инициализация контекста с возможностью отмены
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Инициализация логгера
	logger, err := zap.NewProduction()
	if err != nil {
		panic("Не удалось инициализировать логгер: " + err.Error())
	}
	defer logger.Sync()

	// Загрузка конфигурации
	cfg, err := config.LoadConfig("configs/config.json")
	if err != nil {
		logger.Fatal("Ошибка загрузки конфигурации", zap.Error(err))
	}

	// Загрузка кошельков
	wallets, err := wallet.LoadWallets("configs/wallets.csv")
	if err != nil {
		logger.Fatal("Ошибка загрузки кошельков", zap.Error(err))
	}

	// Инициализация клиента Solana
	solanaClient, err := solanaclient.NewClient(cfg.RPCList, logger)
	if err != nil {
		logger.Fatal("Ошибка инициализации клиента Solana", zap.Error(err))
	}

	// Загрузка задач
	tasks, err := sniping.LoadTasks("configs/tasks.csv")
	if err != nil {
		logger.Fatal("Ошибка загрузки задач", zap.Error(err))
	}

	// Создание экземпляра снайпера
	sniper := sniping.NewSniper(solanaClient, wallets, cfg, logger)

	// Запуск снайпера с загруженными задачами в отдельной горутине
	go func() {
		sniper.Run(ctx, tasks)
	}()

	// Ожидание сигнала завершения
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-sigCh:
		logger.Info("Получен сигнал завершения")
	case <-ctx.Done():
		logger.Info("Контекст отменен")
	}

	// Graceful shutdown
	logger.Info("Начало graceful shutdown")
	cancel()
	// Здесь можно добавить дополнительную логику завершения, если необходимо
	logger.Info("Бот успешно завершил работу")
}
