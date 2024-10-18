package sniping

import (
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
	// Создаем новый экземпляр Sniper
	return &Sniper{
		client:  client,
		wallets: wallets,
		config:  cfg,
		logger:  logger,
	}
}

func (s *Sniper) Run(tasks []*Task) {
	// Проходим по списку задач
	for _, task := range tasks {
		// Запускаем каждую задачу в отдельной горутине или по количеству Workers
		go s.ExecuteTask(task)
	}

	// Ожидание завершения всех задач или обработка сигналов завершения
}
