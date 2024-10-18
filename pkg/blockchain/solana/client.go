package solana

import (
	"github.com/gagliardetto/solana-go"
	"go.uber.org/zap"
)

type Client struct {
	rpcPool *RPCPool
	logger  *zap.Logger
}

func NewClient(rpcList []string, logger *zap.Logger) *Client {
	// Создаем новый клиент Solana с пулом RPC
	rpcPool := NewRPCPool(rpcList)
	return &Client{
		rpcPool: rpcPool,
		logger:  logger,
	}
}

func (c *Client) SendTransaction(tx *solana.Transaction) (string, error) {
	// Получаем доступный RPC-клиент из пула
	rpcClient := c.rpcPool.GetClient()

	// Отправляем транзакцию через RPC-клиент
	txHash, err := rpcClient.SendTransaction(tx)
	if err != nil {
		// Логируем и возвращаем ошибку
		c.logger.Error("Ошибка отправки транзакции", zap.Error(err))
		return "", err
	}

	return txHash, nil
}

func (c *Client) GetRecentBlockhash() (string, error) {
	// Получаем последний blockhash из сети
	rpcClient := c.rpcPool.GetClient()
	blockhash, err := rpcClient.GetRecentBlockhash()
	if err != nil {
		// Логируем и возвращаем ошибку
		c.logger.Error("Ошибка получения blockhash", zap.Error(err))
		return "", err
	}

	return blockhash, nil
}

func (c *Client) SubscribeNewPools(handler func(poolID string)) error {
	// Подписываемся на события создания новых пулов
	// Используем WebSocket или RPC подписки
	// При обнаружении нового пула вызываем handler(poolID)
	return nil
}
