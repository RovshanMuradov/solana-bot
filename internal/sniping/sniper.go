package sniping

import (
	"context"
	"sync"

	solanaBlockchain "github.com/rovshanmuradov/solana-bot/internal/blockchain/solana"
	"github.com/rovshanmuradov/solana-bot/internal/config"
	"github.com/rovshanmuradov/solana-bot/internal/dex"
	"github.com/rovshanmuradov/solana-bot/internal/dex/raydium"
	"github.com/rovshanmuradov/solana-bot/internal/types"
	"github.com/rovshanmuradov/solana-bot/internal/wallet"
	"go.uber.org/zap"
)

type Sniper struct {
	blockchains map[string]types.Blockchain
	wallets     map[string]*wallet.Wallet
	config      *config.Config
	logger      *zap.Logger
	client      *solanaBlockchain.Client // Добавляем клиент Solana
}

func NewSniper(blockchains map[string]types.Blockchain, wallets map[string]*wallet.Wallet, cfg *config.Config, logger *zap.Logger, client *solanaBlockchain.Client) *Sniper {
	return &Sniper{
		blockchains: blockchains,
		wallets:     wallets,
		config:      cfg,
		logger:      logger,
		client:      client,
	}
}

func (s *Sniper) Run(ctx context.Context, tasks []*types.Task) {
	var wg sync.WaitGroup
	taskChan := make(chan *types.Task, len(tasks))

	workers := s.config.Workers
	if workers <= 0 {
		workers = 1 // Устанавливаем минимальное значение, если в конфигурации указано некорректное
		s.logger.Warn("Invalid workers count in config, using 1 worker")
	}

	// Запуск воркеров
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go s.worker(ctx, &wg, taskChan)
	}

	// Отправка задач в канал
	for _, task := range tasks {
		taskChan <- task
	}
	close(taskChan)

	// Ожидание завершения всех горутин
	wg.Wait()
	s.logger.Info("Sniper завершил работу")
}

func (s *Sniper) worker(ctx context.Context, wg *sync.WaitGroup, taskChan <-chan *types.Task) {
	defer wg.Done()

	for task := range taskChan {
		s.executeTask(ctx, task)
	}
}

func (s *Sniper) executeTask(ctx context.Context, task *types.Task) {
	s.logger.Info("Начало выполнения задачи", zap.String("task", task.TaskName))

	// Получаем DEX-модуль на основе имени
	dexModule, err := dex.GetDEXByName(task.DEXName, s.client, s.logger)
	if err != nil {
		s.logger.Error("Не удалось получить DEX-модуль", zap.Error(err))
		return
	}

	// Приводим к конкретному типу, если необходимо
	raydiumDEX, ok := dexModule.(*raydium.RaydiumDEX)
	if !ok {
		s.logger.Error("Неверный тип DEX-модуля")
		return
	}

	// Получаем кошелек
	wallet, ok := s.wallets[task.WalletName]
	if !ok {
		s.logger.Error("Кошелек не найден", zap.String("wallet", task.WalletName))
		return
	}

	// Выполняем свап
	err = raydiumDEX.ExecuteSwap(ctx, task, wallet)
	if err != nil {
		s.logger.Error("Ошибка при выполнении свапа", zap.Error(err))
		return
	}

	s.logger.Info("Свап успешно выполнен")
}

func (s *Sniper) processTask(task *types.Task) error {
	// Реализация логики обработки задачи
	// TODO: Добавить конкретную логику в зависимости от типа задачи
	return nil
}
