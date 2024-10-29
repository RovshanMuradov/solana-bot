// internal/blockchain/solbc/client.go
package solbc

import (
	"context"
	"fmt"

	"github.com/gagliardetto/solana-go"
	solanarpc "github.com/gagliardetto/solana-go/rpc"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc/rpc"
	"go.uber.org/zap"
)

// NewClient создает новый экземпляр клиента с улучшенным мониторингом
func NewClient(rpcURLs []string, logger *zap.Logger) (*Client, error) {
	logger = logger.Named("solana-client")

	// Создаем новый RPC клиент
	rpcClient, err := rpc.NewClient(rpcURLs, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create RPC client: %w", err)
	}

	return &Client{
		rpc:     rpcClient,
		logger:  logger,
		metrics: &ClientMetrics{},
	}, nil
}

// GetAccountInfo получает информацию об аккаунте с расширенной диагностикой
func (c *Client) GetAccountInfo(ctx context.Context, pubkey solana.PublicKey) (*solanarpc.GetAccountInfoResult, error) {
	result, err := c.rpc.GetAccountInfo(ctx, pubkey)
	if err != nil {
		c.metrics.FailedRequests++
		return nil, err
	}
	c.metrics.AccountInfoRequests++
	return result, nil
}

// GetRecentBlockhash получает последний блокхеш с повторными попытками
func (c *Client) GetRecentBlockhash(ctx context.Context) (solana.Hash, error) {
	result, err := c.rpc.GetLatestBlockhash(ctx)
	if err != nil {
		return solana.Hash{}, err
	}
	return result.Value.Blockhash, nil
}

// SendTransaction отправляет транзакцию
// SendTransaction отправляет транзакцию с улучшенной обработкой ошибок
func (c *Client) SendTransaction(ctx context.Context, tx *solana.Transaction) (solana.Signature, error) {
	signature, err := c.rpc.SendTransaction(ctx, tx)
	if err != nil {
		c.metrics.FailedRequests++
		return solana.Signature{}, err
	}
	c.metrics.TransactionRequests++
	return signature, nil
}

func (c *Client) SendTransactionWithOpts(
	ctx context.Context,
	tx *solana.Transaction,
	opts blockchain.TransactionOptions,
) (solana.Signature, error) {
	rpcOpts := rpc.SendTransactionOpts{
		SkipPreflight:       opts.SkipPreflight,
		PreflightCommitment: opts.PreflightCommitment,
	}
	return c.rpc.SendTransactionWithOpts(ctx, tx, rpcOpts)
}

func (c *Client) Close() error {
	c.rpc.Close()
	return nil
}
func (c *Client) GetSignatureStatuses(ctx context.Context, signatures ...solana.Signature) (*solanarpc.GetSignatureStatusesResult, error) {
	return c.rpc.GetSignatureStatuses(ctx, signatures...)
}
