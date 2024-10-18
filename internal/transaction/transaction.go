package transaction

import (
	"github.com/gagliardetto/solana-go"
	"github.com/rovshanmuradov/solana-bot/internal/sniping"
	"github.com/rovshanmuradov/solana-bot/internal/wallet"
	"go.uber.org/zap"
)

func PrepareTransaction(task *sniping.Task, wallet *wallet.Wallet) (*solana.Transaction, error) {
	// Создаем новую транзакцию
	tx := solana.NewTransaction()

	// Добавляем инструкции для обмена SOL на целевые токены
	// Учитываем параметры из задачи (AmountIn, MinAmountOut и т.д.)

	// Подписываем транзакцию приватным ключом кошелька
	err := wallet.SignTransaction(tx)
	if err != nil {
		// Возвращаем ошибку подписания
		return nil, err
	}

	return tx, nil
}

func SendTransaction(tx *solana.Transaction, client *solana.Client, logger *zap.Logger) (string, error) {
	// Отправляем транзакцию в сеть Solana
	txHash, err := client.SendTransaction(tx)
	if err != nil {
		// Логируем и возвращаем ошибку отправки
		logger.Error("Ошибка отправки транзакции", zap.Error(err))
		return "", err
	}

	// Возвращаем хеш транзакции
	return txHash, nil
}
