package sniping

import (
	"context"
	"time"

	"github.com/rovshanmuradov/solana-bot/internal/config"
	"github.com/rovshanmuradov/solana-bot/internal/wallet"
	"github.com/rovshanmuradov/solana-bot/pkg/blockchain/solana"
	"go.uber.org/zap"
)

type Sniper struct {
	client  *solana.Client
	wallets map[string]*wallet.Wallet
	config  *config.Config
	logger  *zap.Logger
}

func NewSniper(client *solana.Client, wallets map[string]*wallet.Wallet, cfg *config.Config, logger *zap.Logger) *Sniper {
	return &Sniper{
		client:  client,
		wallets: wallets,
		config:  cfg,
		logger:  logger,
	}
}

func (s *Sniper) Run(ctx context.Context, tasks []*Task) {
	for _, task := range tasks {
		go s.executeTask(ctx, task)
	}

	<-ctx.Done()
	s.logger.Info("Sniper завершил работу")
}

func (s *Sniper) executeTask(ctx context.Context, task *Task) {
	s.logger.Info("Начало выполнения задачи", zap.String("task", task.TaskName))

	ticker := time.NewTicker(time.Duration(s.config.MonitorDelay) * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("Задача отменена", zap.String("task", task.TaskName))
			return
		case <-ticker.C:
			if err := s.processTask(task); err != nil {
				s.logger.Error("Ошибка при обработке задачи", zap.Error(err), zap.String("task", task.TaskName))
			}
		}
	}
}

func (s *Sniper) processTask(task *Task) error {
	// Здесь реализуйте логику обработки задачи
	// Например, проверка условий, выполнение транзакций и т.д.
	return nil
}
