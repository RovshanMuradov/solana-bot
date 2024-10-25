// internal/sniping/sniper.go
package sniping

import (
	"context"
	"fmt"
	"sync"

	solanaBlockchain "github.com/rovshanmuradov/solana-bot/internal/blockchain/solana"
	"github.com/rovshanmuradov/solana-bot/internal/config"
	"github.com/rovshanmuradov/solana-bot/internal/dex"
	"github.com/rovshanmuradov/solana-bot/internal/storage"
	"github.com/rovshanmuradov/solana-bot/internal/types"
	"github.com/rovshanmuradov/solana-bot/internal/wallet"
	"go.uber.org/zap"
)

type Sniper struct {
	blockchains map[string]types.Blockchain
	wallets     map[string]*wallet.Wallet
	config      *config.Config
	logger      *zap.Logger
	client      *solanaBlockchain.Client
	storage     storage.Storage // Добавляем поле storage
}

func NewSniper(
	blockchains map[string]types.Blockchain,
	wallets map[string]*wallet.Wallet,
	cfg *config.Config,
	logger *zap.Logger,
	client *solanaBlockchain.Client,
	storage storage.Storage, // Добавляем параметр storage
) *Sniper {
	return &Sniper{
		blockchains: blockchains,
		wallets:     wallets,
		config:      cfg,
		logger:      logger,
		client:      client,
		storage:     storage,
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
	fmt.Printf("\nDEBUG: Starting executeTask\n")
	fmt.Printf("DEBUG: Task: %+v\n", task)
	fmt.Printf("DEBUG: Client nil? %v\n", s.client == nil)
	fmt.Printf("DEBUG: Logger nil? %v\n", s.logger == nil)

	s.logger.Info("Starting task execution",
		zap.String("task_name", task.TaskName),
		zap.String("dex_name", task.DEXName),
		zap.String("wallet", task.WalletName))

	// Проверяем наличие и соответствие DEX имени и модуля
	if task.DEXName == "" {
		task.DEXName = task.Module // Используем модуль как DEX имя если DEXName пустой
		fmt.Printf("DEBUG: Using module as DEX name: %s\n", task.DEXName)
	}

	// Получаем DEX-модуль
	fmt.Printf("DEBUG: Getting DEX module for: %s\n", task.DEXName)
	dexModule, err := dex.GetDEXByName(task.DEXName, s.client, s.logger)
	if err != nil {
		s.logger.Error("Failed to get DEX module",
			zap.String("dex_name", task.DEXName),
			zap.Error(err))
		fmt.Printf("DEBUG: Failed to get DEX module: %v\n", err)
		return
	}

	if dexModule == nil {
		s.logger.Error("DEX module is nil after initialization")
		fmt.Printf("DEBUG: DEX module is nil after initialization\n")
		return
	}

	fmt.Printf("DEBUG: Got DEX module of type: %T\n", dexModule)

	s.logger.Info("Got DEX module",
		zap.String("dex_name", task.DEXName),
		zap.String("dex_type", fmt.Sprintf("%T", dexModule)))

	// Получаем и проверяем кошелек
	wallet, ok := s.wallets[task.WalletName]
	if !ok {
		s.logger.Error("Wallet not found",
			zap.String("wallet_name", task.WalletName),
			zap.Strings("available_wallets", getWalletNames(s.wallets)))
		return
	}

	if wallet == nil {
		s.logger.Error("Wallet is nil",
			zap.String("wallet_name", task.WalletName))
		return
	}

	s.logger.Info("Got wallet",
		zap.String("wallet_name", task.WalletName),
		zap.String("public_key", wallet.PublicKey.String()))

	// Проверяем параметры
	if err := validateTaskParameters(task); err != nil {
		s.logger.Error("Invalid task parameters", zap.Error(err))
		return
	}

	// Выполняем свап
	s.logger.Debug("Executing swap")
	if err := dexModule.ExecuteSwap(ctx, task, wallet); err != nil {
		s.logger.Error("Failed to execute swap", zap.Error(err))
		return
	}

	s.logger.Info("Task completed successfully",
		zap.String("task_name", task.TaskName))
}

// Вспомогательная функция для получения имен кошельков
func getWalletNames(wallets map[string]*wallet.Wallet) []string {
	names := make([]string, 0, len(wallets))
	for name := range wallets {
		names = append(names, name)
	}
	return names
}

func validateTaskParameters(task *types.Task) error {
	if task.AmountIn <= 0 {
		return fmt.Errorf("invalid amount_in: %f", task.AmountIn)
	}
	if task.MinAmountOut <= 0 {
		return fmt.Errorf("invalid min_amount_out: %f", task.MinAmountOut)
	}
	return nil
}
