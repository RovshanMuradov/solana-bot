package sniping

import (
	"context"
	"sync"

	"github.com/gagliardetto/solana-go"
	solanaBlockchain "github.com/rovshanmuradov/solana-bot/internal/blockchain/solana"
	"github.com/rovshanmuradov/solana-bot/internal/config"
	"github.com/rovshanmuradov/solana-bot/internal/dex"
	"github.com/rovshanmuradov/solana-bot/internal/types"
	"github.com/rovshanmuradov/solana-bot/internal/wallet"
	"go.uber.org/zap"
)

type Sniper struct {
	blockchains map[string]types.Blockchain
	wallets     map[string]*wallet.Wallet
	config      *config.Config
	logger      *zap.Logger
}

func NewSniper(blockchains map[string]types.Blockchain, wallets map[string]*wallet.Wallet, cfg *config.Config, logger *zap.Logger) *Sniper {
	return &Sniper{
		blockchains: blockchains,
		wallets:     wallets,
		config:      cfg,
		logger:      logger,
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
	dexModule, err := dex.GetDEXByName(task.DEXName)
	if err != nil {
		s.logger.Error("Не удалось получить DEX-модуль", zap.Error(err))
		return
	}

	// Используем DEX-модуль для подготовки инструкции свапа
	instruction, err := dexModule.PrepareSwapInstruction(
		ctx,
		s.wallets[task.WalletName].PublicKey,
		solana.MustPublicKeyFromBase58(task.SourceToken),
		solana.MustPublicKeyFromBase58(task.TargetToken),
		uint64(task.AmountIn),
		uint64(task.MinAmountOut),
		s.logger,
	)
	if err != nil {
		s.logger.Error("Ошибка при подготовке инструкции свапа", zap.Error(err))
		return
	}

	// Теперь используем instruction для создания и отправки транзакции
	solanaBC, ok := s.blockchains["Solana"].(*solanaBlockchain.SolanaBlockchain)
	if !ok {
		s.logger.Error("Неверный тип блокчейна Solana")
		return
	}

	recentBlockhash, err := solanaBC.GetRecentBlockhash(ctx)
	if err != nil {
		s.logger.Error("Ошибка при получении recent blockhash", zap.Error(err))
		return
	}

	tx, err := solana.NewTransaction(
		[]solana.Instruction{instruction},
		recentBlockhash,
	)
	if err != nil {
		s.logger.Error("Ошибка при создании транзакции", zap.Error(err))
		return
	}
	// Подписываем транзакцию
	err = s.wallets[task.WalletName].SignTransaction(tx)
	if err != nil {
		s.logger.Error("Ошибка при подписании транзакции", zap.Error(err))
		return
	}

	// Отправляем транзакцию
	signature, err := s.blockchains["Solana"].SendTransaction(ctx, tx)
	if err != nil {
		s.logger.Error("Ошибка при отправке транзакции", zap.Error(err))
		return
	}

	s.logger.Info("Транзакция успешно отправлена", zap.String("signature", signature))
}

func (s *Sniper) processTask(task *types.Task) error {
	// Реализация логики обработки задачи
	// TODO: Добавить конкретную логику в зависимости от типа задачи
	return nil
}
