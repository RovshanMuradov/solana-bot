// internal/sniping/sniper.go
package sniping

import (
	"context"
	"fmt"
	"sync"

	"github.com/gagliardetto/solana-go"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc" // Добавляем импорт
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
	client      blockchain.Client
	storage     storage.Storage
	tokenCache  *solbc.TokenMetadataCache
}

func NewSniper(
	blockchains map[string]types.Blockchain,
	wallets map[string]*wallet.Wallet,
	cfg *config.Config,
	logger *zap.Logger,
	client blockchain.Client,
	storage storage.Storage,
) *Sniper {
	return &Sniper{
		blockchains: blockchains,
		wallets:     wallets,
		config:      cfg,
		logger:      logger,
		client:      client,
		storage:     storage,
		tokenCache:  solbc.NewTokenMetadataCache(logger),
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
	s.logger.Info("Starting task execution",
		zap.String("task_name", task.TaskName),
		zap.String("dex_name", task.DEXName),
		zap.String("wallet", task.WalletName))

	// Устанавливаем DEXName если пустой
	if task.DEXName == "" {
		task.DEXName = task.Module
	}

	// Получаем DEX-модуль
	dexModule, err := dex.GetDEXByName(task.DEXName, s.client, s.logger)
	if err != nil {
		s.logger.Error("Failed to get DEX module",
			zap.String("dex_name", task.DEXName),
			zap.Error(err))
		return
	}

	// Получаем публичные ключи токенов
	sourceMint, err := solana.PublicKeyFromBase58(task.SourceToken)
	if err != nil {
		s.logger.Error("Invalid source token address", zap.Error(err))
		return
	}

	targetMint, err := solana.PublicKeyFromBase58(task.TargetToken)
	if err != nil {
		s.logger.Error("Invalid target token address", zap.Error(err))
		return
	}

	// Получаем метаданные обоих токенов параллельно
	metadata, err := s.tokenCache.GetMultipleTokenMetadata(ctx, s.client, []solana.PublicKey{sourceMint, targetMint})
	if err != nil {
		s.logger.Error("Failed to get token metadata", zap.Error(err))
		return
	}

	// Устанавливаем decimals из метаданных
	sourceMetadata := metadata[sourceMint.String()]
	targetMetadata := metadata[targetMint.String()]

	task.SourceTokenDecimals = int(sourceMetadata.Decimals)
	task.TargetTokenDecimals = int(targetMetadata.Decimals)

	s.logger.Debug("Token metadata loaded",
		zap.String("source_symbol", sourceMetadata.Symbol),
		zap.Int("source_decimals", task.SourceTokenDecimals),
		zap.String("target_symbol", targetMetadata.Symbol),
		zap.Int("target_decimals", task.TargetTokenDecimals))

	// Получаем и проверяем кошелек
	wallet, ok := s.wallets[task.WalletName]
	if !ok || wallet == nil {
		s.logger.Error("Wallet not found or invalid",
			zap.String("wallet_name", task.WalletName))
		return
	}

	// Проверяем параметры
	if err := validateTaskParameters(task); err != nil {
		s.logger.Error("Invalid task parameters", zap.Error(err))
		return
	}

	// Выполняем свап
	if err := dexModule.ExecuteSwap(ctx, task, wallet); err != nil {
		s.logger.Error("Swap execution failed",
			zap.String("task", task.TaskName),
			zap.Error(err))
		return
	}

	s.logger.Info("Task completed successfully",
		zap.String("task_name", task.TaskName))
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
