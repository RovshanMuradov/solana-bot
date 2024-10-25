// pkg/blockchain/solana/client.go
package solana

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"go.uber.org/zap"
)

// NewClient создает новый экземпляр клиента Solana с пулом RPC узлов
func NewClient(rpcList []string, logger *zap.Logger) (*Client, error) {
	if len(rpcList) == 0 {
		return nil, errors.New("empty RPC list")
	}

	var rpcPool []*RPCClient

	for _, rpcURL := range rpcList {
		if _, err := url.Parse(rpcURL); err != nil {
			return nil, errors.New("invalid RPC URL: " + rpcURL)
		}

		rpcPool = append(rpcPool, &RPCClient{
			Client: rpc.New(rpcURL),
			RPCURL: rpcURL,
		})
	}

	logger.Debug("Initializing Solana client", zap.Strings("rpc_urls", rpcList))

	// Попробуем подключиться к каждому узлу с таймаутом
	var lastErr error
	for _, rpcClient := range rpcPool {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := testConnection(ctx, rpcClient.Client, rpcClient.RPCURL, logger); err != nil {
			logger.Warn("Failed to connect to RPC node",
				zap.Error(err),
				zap.String("rpc_url", rpcClient.RPCURL))
			lastErr = err
		} else {
			// Успешное подключение
			logger.Info("Successfully connected to RPC node", zap.String("rpc_url", rpcClient.RPCURL))
			return &Client{
				rpcPool: rpcPool,
				logger:  logger,
			}, nil
		}
	}

	// Если все узлы недоступны, возвращаем последнюю ошибку
	return nil, fmt.Errorf("all RPC nodes failed: %w", lastErr)
}

func testConnection(ctx context.Context, client *rpc.Client, rpcURL string, logger *zap.Logger) error {
	logger.Debug("Testing RPC connection", zap.String("rpc_url", rpcURL))

	res, err := client.GetRecentBlockhash(ctx, rpc.CommitmentFinalized)
	if err != nil {
		logger.Error("RPC connection test failed",
			zap.Error(err),
			zap.String("method", "GetRecentBlockhash"))
		return fmt.Errorf("failed to get recent blockhash: %w", err)
	}

	if res.Value.Blockhash.IsZero() {
		logger.Error("Invalid blockhash received",
			zap.String("blockhash", res.Value.Blockhash.String()))
		return errors.New("received zero blockhash")
	}

	logger.Debug("RPC connection successful",
		zap.String("rpc_url", rpcURL),
		zap.String("blockhash", res.Value.Blockhash.String()),
		zap.Uint64("fee_calculator.lamports_per_signature", res.Value.FeeCalculator.LamportsPerSignature))
	return nil
}

// Остальной код остается без изменений

func (c *Client) SendTransaction(ctx context.Context, tx *solana.Transaction) (solana.Signature, error) {
	rpcClient := c.getClient()
	txHash, err := rpcClient.Client.SendTransactionWithOpts(ctx, tx, rpc.TransactionOpts{
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
	rpcClient := c.getClient()
	result, err := rpcClient.Client.GetRecentBlockhash(ctx, rpc.CommitmentFinalized)
	if err != nil {
		c.logger.Error("Ошибка получения blockhash", zap.Error(err))
		return solana.Hash{}, err
	}
	return result.Value.Blockhash, nil
}

// Метод для получения клиента из пула
func (c *Client) getClient() *RPCClient {
	// Логика для выбора клиента, например, по круговому алгоритму или случайному выбору
	return c.rpcPool[0] // Здесь возвращается первый клиент, но это можно улучшить
}
