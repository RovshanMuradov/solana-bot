package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc"
	"github.com/rovshanmuradov/solana-bot/internal/config"
	"github.com/rovshanmuradov/solana-bot/internal/dex"
	"github.com/rovshanmuradov/solana-bot/internal/eventlistener"
	"github.com/rovshanmuradov/solana-bot/internal/storage/postgres"
	"github.com/rovshanmuradov/solana-bot/internal/utils/metrics"
	"github.com/rovshanmuradov/solana-bot/internal/wallet"
	"go.uber.org/zap"
)

// Функция run содержит основную логику приложения и возвращает ошибку в случае неудачи.
func run() error {
	// Создаём корневой контекст для управления временем жизни приложения
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Инициализация логгера
	logger, err := zap.NewDevelopment()
	if err != nil {
		return fmt.Errorf("ошибка инициализации логгера: %w", err)
	}
	// Проверяем ошибку при закрытии логгера
	defer func() {
		if err := logger.Sync(); err != nil {
			fmt.Printf("Ошибка при закрытии логгера: %v\n", err)
		}
	}()
	logger.Info("Запуск бота")

	// Инициализация сборщика метрик
	metricsCollector := metrics.NewCollector()

	// Запуск HTTP-сервера для экспорта метрик (например, для Prometheus)
	go func() {
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		srv := &http.Server{
			Addr:         ":2112",
			Handler:      mux,
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 10 * time.Second,
			IdleTimeout:  15 * time.Second,
		}
		logger.Info("Метрики доступны на :2112/metrics")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("Ошибка HTTP-сервера метрик", zap.Error(err))
		}
	}()

	// Загрузка конфигурации из файла
	cfg, err := config.LoadConfig("configs/config.json")
	if err != nil {
		return fmt.Errorf("не удалось загрузить конфигурацию: %w", err)
	}
	logger.Info("Конфигурация загружена", zap.Any("config", cfg))

	// Загрузка кошельков из CSV-файла
	walletsMap, err := wallet.LoadWallets("configs/wallets.csv")
	if err != nil {
		return fmt.Errorf("не удалось загрузить кошельки: %w", err)
	}
	logger.Info("Кошельки загружены", zap.Int("количество", len(walletsMap)))

	// Выбираем первый доступный кошелек (при необходимости можно расширить логику выбора)
	var primaryWallet *wallet.Wallet
	for _, w := range walletsMap {
		primaryWallet = w
		break
	}
	if primaryWallet == nil {
		return fmt.Errorf("нет доступных кошельков")
	}
	logger.Info("Используется кошелек", zap.String("publicKey", primaryWallet.PublicKey.String()))

	// Инициализация клиента Solana с использованием первого RPC из списка
	if len(cfg.RPCList) == 0 {
		return fmt.Errorf("список RPC пуст")
	}
	solClient := solbc.NewClient(cfg.RPCList[0], logger)
	// Если клиент имеет метод Close, его можно вызвать при завершении работы, например:
	// defer solClient.Close()

	// Инициализация Postgres-хранилища
	store, err := postgres.NewStorage(cfg.PostgresURL, logger)
	if err != nil {
		return fmt.Errorf("не удалось инициализировать Postgres-хранилище: %w", err)
	}
	logger.Info("Postgres-хранилище инициализировано")

	// Выполнение миграций базы данных
	if err := store.RunMigrations(); err != nil {
		return fmt.Errorf("ошибка при выполнении миграций: %w", err)
	}
	logger.Info("Миграции базы данных выполнены успешно")

	// Инициализация DEX‑адаптера (выбираем, например, "pump.fun")
	dexName := "pump.fun"
	dexAdapter, err := dex.GetDEXByName(dexName, solClient, primaryWallet, logger, metricsCollector)
	if err != nil {
		return fmt.Errorf("ошибка инициализации DEX адаптера: %w", err)
	}
	logger.Info("DEX адаптер инициализирован", zap.String("DEX", dexAdapter.GetName()))

	// Инициализация слушателя событий по WebSocket
	if cfg.WebSocketURL == "" {
		return fmt.Errorf("WebSocketURL не задан в конфигурации")
	}
	eventListener, err := eventlistener.NewEventListener(ctx, cfg.WebSocketURL, logger)
	if err != nil {
		return fmt.Errorf("ошибка инициализации слушателя событий: %w", err)
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

		// Дополнительно можно сохранить информацию о задаче в БД через store
	}

	// Запускаем подписку на события в отдельной горутине
	go func() {
		if err := eventListener.Subscribe(ctx, eventHandler); err != nil {
			// Если произошла фатальная ошибка при подписке, завершаем приложение
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
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Printf("Ошибка: %v\n", err)
		os.Exit(1)
	}
}
