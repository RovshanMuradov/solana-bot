package transaction

import (
	"context"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/rovshanmuradov/solana-bot/internal/sniping"
	"github.com/rovshanmuradov/solana-bot/internal/wallet"
	solanaclient "github.com/rovshanmuradov/solana-bot/pkg/blockchain/solana"
	"go.uber.org/zap"
)

func PrepareTransaction(ctx context.Context, task *sniping.Task, wallet *wallet.Wallet, client *solanaclient.Client) (*solana.Transaction, error) {
	// Получаем последний blockhash
	blockhash, err := client.GetRecentBlockhash(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get recent blockhash: %w", err)
	}

	// Создаем новую транзакцию с полученным blockhash
	tx, err := solana.NewTransaction(
		[]solana.Instruction{},
		blockhash,
		solana.TransactionPayer(wallet.PublicKey),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create new transaction: %w", err)
	}

	// Здесь нужно добавить инструкции для обмена SOL на целевые токены
	// Учитываем параметры из задачи (AmountIn, MinAmountOut и т.д.)
	// Например, создаем инструкцию для обмена
	// instr := ... // реализация инструкции обмена
	// tx.Add(instr)

	// Подписываем транзакцию приватным ключом кошелька
	_, err = tx.Sign(func(key solana.PublicKey) *solana.PrivateKey {
		if wallet.PublicKey.Equals(key) {
			return &wallet.PrivateKey
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to sign transaction: %w", err)
	}

	return tx, nil
}

func SendTransaction(ctx context.Context, tx *solana.Transaction, client *solanaclient.Client, logger *zap.Logger) (solana.Signature, error) {
	// Отправляем транзакцию в сеть Solana
	txHash, err := client.SendTransaction(ctx, tx)
	if err != nil {
		// Логируем и возвращаем ошибку отправки
		logger.Error("Ошибка отправки транзакции", zap.Error(err))
		return solana.Signature{}, err
	}

	// Возвращаем хеш транзакции
	return txHash, nil
}
