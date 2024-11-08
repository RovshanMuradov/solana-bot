// internal/sniping/sniper.go

package sniping

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc"
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
	// Добавляем каналы для управления
	taskChan  chan *types.Task
	doneChan  chan struct{}
	errorChan chan error
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
		logger:      logger.Named("sniper"),
		client:      client,
		storage:     storage,
		tokenCache:  solbc.NewTokenMetadataCache(logger),
		taskChan:    make(chan *types.Task, 100),
		doneChan:    make(chan struct{}),
		errorChan:   make(chan error, 100),
	}
}

func (s *Sniper) Run(ctx context.Context, tasks []*types.Task) error {
	// Создаем контекст с отменой
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	workers := s.config.Workers
	if workers <= 0 {
		workers = 1
	}

	// Запускаем воркеров
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go s.worker(ctx, &wg, i)
	}

	// Запускаем горутину для мониторинга ошибок
	go s.monitorErrors(ctx)

	// Отправляем задачи в канал
	go func() {
		for _, task := range tasks {
			select {
			case <-ctx.Done():
				return
			case s.taskChan <- task:
				s.logger.Debug("Task queued",
					zap.String("task_name", task.TaskName),
					zap.Int("workers", workers))
			}
		}
		close(s.taskChan)
	}()

	// Ожидаем завершения всех воркеров
	wg.Wait()
	close(s.doneChan)
	close(s.errorChan)

	s.logger.Info("Sniper finished work")
	return nil
}

func (s *Sniper) worker(ctx context.Context, wg *sync.WaitGroup, id int) {
	defer wg.Done()

	for {
		select {
		case <-ctx.Done():
			s.logger.Debug("Worker stopped due to context cancellation",
				zap.Int("worker_id", id))
			return

		case task, ok := <-s.taskChan:
			if !ok {
				s.logger.Debug("Task channel closed, worker stopping",
					zap.Int("worker_id", id))
				return
			}

			// Создаем таймаут для выполнения задачи
			taskCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			err := s.executeTask(taskCtx, task)
			cancel()

			if err != nil {
				s.logger.Error("Task execution failed",
					zap.String("task_name", task.TaskName),
					zap.Error(err))
				select {
				case s.errorChan <- err:
				case <-ctx.Done():
					return
				}
			}

			// Добавляем задержку между задачами
			if task.TransactionDelay > 0 {
				select {
				case <-time.After(time.Duration(task.TransactionDelay) * time.Millisecond):
				case <-ctx.Done():
					return
				}
			}
		}
	}
}

func (s *Sniper) monitorErrors(ctx context.Context) {
	var errorCount int
	for {
		select {
		case <-ctx.Done():
			return
		case err, ok := <-s.errorChan:
			if !ok {
				return
			}
			errorCount++
			s.logger.Error("Task error received",
				zap.Error(err),
				zap.Int("total_errors", errorCount))
		}
	}
}

func (s *Sniper) executeTask(ctx context.Context, task *types.Task) error {
	s.logger.Info("Starting task execution",
		zap.String("task_name", task.TaskName),
		zap.String("dex_name", task.DEXName),
		zap.String("wallet", task.WalletName))

	// Проверяем контекст перед каждой длительной операцией
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context cancelled before task execution: %w", err)
	}

	// Получаем DEX
	dexModule, err := dex.GetDEXByName(task.DEXName, s.client, s.logger)
	if err != nil {
		return fmt.Errorf("failed to get DEX module: %w", err)
	}

	// Валидируем токены
	sourceMint, targetMint, err := s.validateTokens(ctx, task)
	if err != nil {
		return fmt.Errorf("token validation failed: %w", err)
	}

	// Получаем кошелек
	wallet, ok := s.wallets[task.WalletName]
	if !ok || wallet == nil {
		return fmt.Errorf("wallet not found: %s", task.WalletName)
	}

	// Получаем метаданные токенов с таймаутом
	tokenCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	metadata, err := s.tokenCache.GetMultipleTokenMetadata(
		tokenCtx,
		s.client,
		[]solana.PublicKey{sourceMint, targetMint},
	)
	if err != nil {
		return fmt.Errorf("failed to get token metadata: %w", err)
	}

	// Устанавливаем decimals из метаданных
	if err := s.updateTaskWithMetadata(task, metadata, sourceMint, targetMint); err != nil {
		return err
	}

	// Выполняем свап
	swapCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	if err := dexModule.ExecuteSwap(swapCtx, task, wallet); err != nil {
		return fmt.Errorf("swap execution failed: %w", err)
	}

	s.logger.Info("Task completed successfully",
		zap.String("task_name", task.TaskName))

	return nil
}

func (s *Sniper) validateTokens(ctx context.Context, task *types.Task) (solana.PublicKey, solana.PublicKey, error) {
	select {
	case <-ctx.Done():
		return solana.PublicKey{}, solana.PublicKey{}, ctx.Err() // обработка отмены контекста
	default:
	}

	sourceMint, err := solana.PublicKeyFromBase58(task.SourceToken)
	if err != nil {
		return solana.PublicKey{}, solana.PublicKey{}, fmt.Errorf("invalid source token: %w", err)
	}

	targetMint, err := solana.PublicKeyFromBase58(task.TargetToken)
	if err != nil {
		return solana.PublicKey{}, solana.PublicKey{}, fmt.Errorf("invalid target token: %w", err)
	}

	return sourceMint, targetMint, nil
}

func (s *Sniper) updateTaskWithMetadata(
	task *types.Task,
	metadata map[string]*solbc.TokenMetadata,
	sourceMint, targetMint solana.PublicKey,
) error {
	sourceMetadata := metadata[sourceMint.String()]
	targetMetadata := metadata[targetMint.String()]

	if sourceMetadata == nil || targetMetadata == nil {
		return fmt.Errorf("metadata not found for tokens")
	}

	task.SourceTokenDecimals = int(sourceMetadata.Decimals)
	task.TargetTokenDecimals = int(targetMetadata.Decimals)

	s.logger.Debug("Token metadata loaded",
		zap.String("source_symbol", sourceMetadata.Symbol),
		zap.Int("source_decimals", task.SourceTokenDecimals),
		zap.String("target_symbol", targetMetadata.Symbol),
		zap.Int("target_decimals", task.TargetTokenDecimals))

	return nil
}
