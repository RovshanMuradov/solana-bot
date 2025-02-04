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
	"github.com/rovshanmuradov/solana-bot/internal/dex"
	"github.com/rovshanmuradov/solana-bot/internal/eventlistener"
	"github.com/rovshanmuradov/solana-bot/internal/storage/postgres"
	"github.com/rovshanmuradov/solana-bot/internal/wallet"
	"go.uber.org/zap"
)

func main() {
	// Создаём корневой контекст для управления временем жизни приложения
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Инициализация логгера (для разработки можно использовать NewDevelopment)
	logger, err := zap.NewDevelopment()
	if err != nil {
		fmt.Printf("Ошибка инициализации логгера: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()
	logger.Info("Запуск бота")

	// Загрузка конфигурации из файла
	cfg, err := config.LoadConfig("configs/config.json")
	if err != nil {
		logger.Fatal("Не удалось загрузить конфигурацию", zap.Error(err))
	}
	logger.Info("Конфигурация загружена", zap.Any("config", cfg))

	// Загрузка кошельков из CSV-файла
	walletsMap, err := wallet.LoadWallets("configs/wallets.csv")
	if err != nil {
		logger.Fatal("Не удалось загрузить кошельки", zap.Error(err))
	}
	logger.Info("Кошельки загружены", zap.Int("количество", len(walletsMap)))

	// Выбираем первый доступный кошелек (можно расширить логику выбора)
	var primaryWallet *wallet.Wallet
	for _, w := range walletsMap {
		primaryWallet = w
		break
	}
	if primaryWallet == nil {
		logger.Fatal("Нет доступных кошельков")
	}
	logger.Info("Используется кошелек", zap.String("publicKey", primaryWallet.PublicKey.String()))

	// Инициализация клиента Solana с использованием первого RPC из списка
	if len(cfg.RPCList) == 0 {
		logger.Fatal("Список RPC пуст")
	}
	solClient := solbc.NewClient(cfg.RPCList[0], logger)
	// Если клиент имеет метод Close, не забудьте его вызывать при завершении работы:
	// defer solClient.Close()

	// Инициализация Postgres-хранилища
	store, err := postgres.NewStorage(cfg.PostgresURL, logger)
	if err != nil {
		logger.Fatal("Не удалось инициализировать Postgres-хранилище", zap.Error(err))
	}
	logger.Info("Postgres-хранилище инициализировано")

	// Выполнение миграций базы данных
	if err := store.RunMigrations(); err != nil {
		logger.Fatal("Ошибка при выполнении миграций", zap.Error(err))
	}
	logger.Info("Миграции базы данных выполнены успешно")

	// Инициализация DEX‑адаптера
	// Выберите нужный DEX, например "pump.fun" или "raydium"
	dexName := "pump.fun"
	dexAdapter, err := dex.GetDEXByName(dexName, solClient, primaryWallet, logger)
	if err != nil {
		logger.Fatal("Ошибка инициализации DEX адаптера", zap.Error(err))
	}
	logger.Info("DEX адаптер инициализирован", zap.String("DEX", dexAdapter.GetName()))

	// Инициализация слушателя событий по WebSocket
	if cfg.WebSocketURL == "" {
		logger.Fatal("WebSocketURL не задан в конфигурации")
	}
	eventListener, err := eventlistener.NewEventListener(ctx, cfg.WebSocketURL, logger)
	if err != nil {
		logger.Fatal("Ошибка инициализации слушателя событий", zap.Error(err))
	}
	logger.Info("Слушатель событий инициализирован", zap.String("WebSocketURL", cfg.WebSocketURL))

	// Определяем обработчик событий
	eventHandler := func(ev eventlistener.Event) {
		logger.Info("Получено событие", zap.String("type", ev.Type), zap.String("pool_id", ev.PoolID))
		switch ev.Type {
		case "NewPool":
			// При появлении нового пула выполняется операция snipe
			task := &dex.Task{
				Operation:    dex.OperationSnipe,
				Amount:       1_000_000_000, // Пример: 1 SOL (в лампортах)
				MinSolOutput: 900_000_000,   // Минимальный ожидаемый вывод
			}
			logger.Info("Выполняется операция snipe")
			if err := dexAdapter.Execute(ctx, task); err != nil {
				logger.Error("Ошибка выполнения snipe", zap.Error(err))
			} else {
				logger.Info("Операция snipe выполнена успешно")
			}
		case "PriceChange":
			// При изменении цены выполняется операция sell
			task := &dex.Task{
				Operation:    dex.OperationSell,
				Amount:       1_000_000_000,
				MinSolOutput: 1_100_000_000,
			}
			logger.Info("Выполняется операция sell")
			if err := dexAdapter.Execute(ctx, task); err != nil {
				logger.Error("Ошибка выполнения sell", zap.Error(err))
			} else {
				logger.Info("Операция sell выполнена успешно")
			}
		default:
			logger.Warn("Неподдерживаемый тип события", zap.String("type", ev.Type))
		}

		// Пример интеграции: можно сохранить информацию о задаче в БД через store
		// taskHistory := &models.TaskHistory{
		//     TaskName: "Название задачи",
		//     Status:   "completed",
		//     // Заполните остальные поля при необходимости...
		// }
		// if err := store.SaveTaskHistory(ctx, taskHistory); err != nil {
		//     logger.Error("Ошибка сохранения истории задачи", zap.Error(err))
		// }
	}

	// Запускаем подписку на события в отдельной горутине
	go func() {
		if err := eventListener.Subscribe(ctx, eventHandler); err != nil {
			logger.Fatal("Ошибка подписки на события", zap.Error(err))
		}
	}()

	// Ожидание системного сигнала для корректного завершения работы
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	logger.Info("Бот запущен. Ожидание сигнала завершения...")
	sig := <-sigCh
	logger.Info("Получен сигнал", zap.String("signal", sig.String()))

	// Завершаем работу: отменяем контекст и даём время на завершение всех операций
	cancel()
	time.Sleep(2 * time.Second)
	logger.Info("Бот успешно завершил работу")
}
