package main

import (
	"github.com/your_project/internal/config"
	"github.com/your_project/internal/sniping"
	"github.com/your_project/internal/wallet"
	"github.com/your_project/pkg/blockchain/solana"
	"go.uber.org/zap"
)

func main() {
	// Инициализация логгера
	logger, err := zap.NewProduction()
	if err != nil {
		// Обработка ошибки и завершение программы
		panic(err)
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
	solanaClient := solana.NewClient(cfg.RPCList, logger)

	// Загрузка задач
	tasks, err := sniping.LoadTasks("configs/tasks.csv")
	if err != nil {
		logger.Fatal("Ошибка загрузки задач", zap.Error(err))
	}

	// Создание экземпляра снайпера
	sniper := sniping.NewSniper(solanaClient, wallets, cfg, logger)

	// Запуск снайпера с загруженными задачами
	sniper.Run(tasks)
}
