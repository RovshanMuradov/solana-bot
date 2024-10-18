package sniping

import (
	"github.com/rovshanmuradov/solana-bot/internal/wallet"
	"go.uber.org/zap"
)

type Task struct {
	TaskName            string
	Module              string
	Workers             int
	WalletName          string
	Delta               int
	PriorityFee         float64
	AMMID               string
	SourceToken         string
	TargetToken         string
	AmountIn            float64
	MinAmountOut        float64
	AutosellPercent     float64
	AutosellDelay       int
	AutosellAmount      float64
	TransactionDelay    int
	AutosellPriorityFee float64
}

func LoadTasks(path string) ([]*Task, error) {
	// Открытие файла tasks.csv
	// Парсинг CSV и создание списка задач
	var tasks []*Task
	// Заполнение списка задач из файла
	return tasks, nil
}

func (s *Sniper) ExecuteTask(task *Task) {
	// Получаем кошелек по имени
	wallet := s.wallets[task.WalletName]

	// В зависимости от модуля выбираем стратегию
	switch task.Module {
	case "RAYDIUM":
		// Запуск стратегии для Raydium
		s.executeRaydiumStrategy(task, wallet)
	case "PUMP.FUN":
		// Запуск стратегии для pump.fun
		s.executePumpFunStrategy(task, wallet)
	default:
		s.logger.Warn("Неизвестный модуль", zap.String("module", task.Module))
	}
}

func (s *Sniper) executeRaydiumStrategy(task *Task, wallet *wallet.Wallet) {
	// Реализация стратегии для Raydium
	// - Подписываемся на новые пулы
	// - При появлении подходящего пула готовим и отправляем транзакцию
}

func (s *Sniper) executePumpFunStrategy(task *Task, wallet *wallet.Wallet) {
	// Реализация стратегии для pump.fun
	// - Аналогично Raydium, но с учетом особенностей pump.fun
}
