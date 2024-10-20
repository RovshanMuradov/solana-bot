package sniping

import (
	"context"
	"sync"
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
	var wg sync.WaitGroup
	taskChan := make(chan *Task, len(tasks))

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

func (s *Sniper) worker(ctx context.Context, wg *sync.WaitGroup, taskChan <-chan *Task) {
	defer wg.Done()

	for task := range taskChan {
		s.executeTask(ctx, task)
	}
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
				// Здесь можно добавить логику повторных попыток или другой обработки ошибок
			}
		}
	}
}

func (s *Sniper) processTask(task *Task) error {
	// Реализация логики обработки задачи
	// TODO: Добавить конкретную логику в зависимости от типа задачи
	return nil
}
