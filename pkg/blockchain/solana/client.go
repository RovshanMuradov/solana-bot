// pkg/blockchain/solana/client.go
package solana

import (
	"context"
	"errors"
	"net/url"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"go.uber.org/zap"
)

type Client struct {
	rpcPool *RPCPool
	logger  *zap.Logger
}

// NewClient создает новый экземпляр клиента Solana
func NewClient(rpcList []string, logger *zap.Logger) (*Client, error) {
	if len(rpcList) == 0 {
		return nil, errors.New("empty RPC list")
	}

	for _, rpcURL := range rpcList {
		if _, err := url.Parse(rpcURL); err != nil {
			return nil, errors.New("invalid RPC URL: " + rpcURL)
		}
	}

	rpcPool := NewRPCPool(rpcList)

	// Попробуем подключиться к первому RPC для проверки
	if err := testConnection(rpcPool.GetClient()); err != nil {
		return nil, err
	}

	return &Client{
		rpcPool: rpcPool,
		logger:  logger,
	}, nil
}

func testConnection(client *rpc.Client) error {
	_, err := client.GetRecentBlockhash(context.Background(), rpc.CommitmentFinalized)
	return err
}

// Остальной код остается без изменений

func (c *Client) SendTransaction(ctx context.Context, tx *solana.Transaction) (solana.Signature, error) {
	rpcClient := c.rpcPool.GetClient()
	txHash, err := rpcClient.SendTransactionWithOpts(ctx, tx, rpc.TransactionOpts{
		SkipPreflight:       true,
		PreflightCommitment: rpc.CommitmentFinalized,
	})
	if err != nil {
		c.logger.Error("Ошибка отправки транзакции", zap.Error(err))
		return solana.Signature{}, err
	}
	return txHash, nil
}

func (c *Client) GetRecentBlockhash(ctx context.Context) (solana.Hash, error) {
	rpcClient := c.rpcPool.GetClient()
	result, err := rpcClient.GetRecentBlockhash(ctx, rpc.CommitmentFinalized)
	if err != nil {
		c.logger.Error("Ошибка получения blockhash", zap.Error(err))
		return solana.Hash{}, err
	}
	return result.Value.Blockhash, nil
}

// Остальной код остается без изменений
