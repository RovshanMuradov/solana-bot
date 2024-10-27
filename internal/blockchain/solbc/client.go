// internal/blockchain/solbc/client.go
package solbc

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/gagliardetto/solana-go"
	solanarpc "github.com/gagliardetto/solana-go/rpc"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc/rpc"
	"go.uber.org/zap"
)

// NewClient создает новый экземпляр клиента Solana
func NewClient(rpcURLs []string, logger *zap.Logger) (*Client, error) {
	if len(rpcURLs) == 0 {
		return nil, fmt.Errorf("empty RPC URL list")
	}

	var clients []*rpc.NodeClient
	for _, urlStr := range rpcURLs {
		if _, err := url.Parse(urlStr); err != nil {
			logger.Warn("Invalid RPC URL", zap.String("url", urlStr), zap.Error(err))
			continue
		}

		client, err := rpc.NewClient(urlStr)
		if err != nil {
			logger.Warn("Failed to create RPC client",
				zap.String("url", urlStr),
				zap.Error(err))
			continue
		}
		clients = append(clients, client)
	}

	if len(clients) == 0 {
		return nil, fmt.Errorf("no valid RPC URLs provided")
	}

	pool := rpc.NewPool(clients, logger)
	client := &Client{
		rpcPool: pool,
		logger:  logger.Named("solana-client"),
	}

	if err := client.validateConnections(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to validate connections: %w", err)
	}

	return client, nil
}

// validateConnections проверяет подключения ко всем RPC узлам
func (c *Client) validateConnections(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, rpc.DefaultTimeout)
	defer cancel()

	var wg sync.WaitGroup
	errChan := make(chan error, len(c.rpcPool.Clients))

	for _, client := range c.rpcPool.Clients {
		wg.Add(1)
		go func(node *rpc.NodeClient) {
			defer wg.Done()

			var lastErr error
			for attempt := 0; attempt < rpc.MaxRetries; attempt++ {
				start := time.Now()
				if err := c.testConnection(ctx, node); err != nil {
					lastErr = err
					node.UpdateMetrics(false, time.Since(start))
					time.Sleep(rpc.RetryDelay)
					continue
				}
				node.UpdateMetrics(true, time.Since(start))
				return
			}
			if lastErr != nil {
				errChan <- rpc.NewError(lastErr, node.URL, "validate_connection")
				node.SetActive(false)
			}
		}(client)
	}

	wg.Wait()
	close(errChan)

	// Собираем все ошибки
	var errors []error
	for err := range errChan {
		errors = append(errors, err)
	}

	// Проверяем наличие активных клиентов
	if !c.rpcPool.HasActiveClients() {
		return fmt.Errorf("no active RPC connections available: %v", errors)
	}

	if len(errors) > 0 {
		c.logger.Warn("Some RPC connections failed",
			zap.Int("total", len(c.rpcPool.Clients)),
			zap.Int("failed", len(errors)))
	}

	return nil
}

// testConnection проверяет подключение к RPC узлу
func (c *Client) testConnection(ctx context.Context, node *rpc.NodeClient) error {
	// Пробуем получить версию узла как более легкий запрос
	version, err := node.Client.GetVersion(ctx)
	if err != nil {
		return fmt.Errorf("failed to get version: %w", err)
	}

	// Если версия получена успешно, пробуем получить последний блокхеш
	_, err = node.Client.GetLatestBlockhash(ctx, solanarpc.CommitmentFinalized)
	if err != nil {
		return fmt.Errorf("failed to get latest blockhash: %w", err)
	}

	c.logger.Debug("Successfully connected to RPC",
		zap.String("url", node.URL),
		zap.String("solana_core", version.SolanaCore))

	return nil
}

// GetAccountInfo получает информацию об аккаунте
func (c *Client) GetAccountInfo(ctx context.Context, pubkey solana.PublicKey) (*solanarpc.GetAccountInfoResult, error) {
	var result *solanarpc.GetAccountInfoResult
	err := c.rpcPool.ExecuteWithRetry(ctx, func(node *rpc.NodeClient) error {
		var err error
		result, err = node.Client.GetAccountInfoWithOpts(ctx, pubkey, &solanarpc.GetAccountInfoOpts{
			Encoding:   solana.EncodingBase64,
			Commitment: solanarpc.CommitmentConfirmed,
		})
		return err
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get account info: %w", err)
	}

	return result, nil
}

// GetRecentBlockhash получает последний блокхеш
func (c *Client) GetRecentBlockhash(ctx context.Context) (solana.Hash, error) {
	var hash solana.Hash
	err := c.rpcPool.ExecuteWithRetry(ctx, func(node *rpc.NodeClient) error {
		result, err := node.Client.GetLatestBlockhash(ctx, solanarpc.CommitmentFinalized)
		if err != nil {
			return err
		}
		hash = result.Value.Blockhash
		return nil
	})

	if err != nil {
		return solana.Hash{}, fmt.Errorf("failed to get recent blockhash: %w", err)
	}

	return hash, nil
}

// SendTransaction отправляет транзакцию
func (c *Client) SendTransaction(ctx context.Context, tx *solana.Transaction) (solana.Signature, error) {
	var signature solana.Signature
	err := c.rpcPool.ExecuteWithRetry(ctx, func(node *rpc.NodeClient) error {
		sig, err := node.Client.SendTransactionWithOpts(ctx, tx, solanarpc.TransactionOpts{
			SkipPreflight:       true,
			PreflightCommitment: solanarpc.CommitmentFinalized,
		})
		if err != nil {
			return err
		}
		signature = sig
		return nil
	})

	if err != nil {
		return solana.Signature{}, fmt.Errorf("failed to send transaction: %w", err)
	}

	return signature, nil
}

// GetMetrics возвращает метрики всех клиентов
func (c *Client) GetMetrics() map[string]struct {
	SuccessCount uint64
	ErrorCount   uint64
	AvgLatency   time.Duration
} {
	metrics := make(map[string]struct {
		SuccessCount uint64
		ErrorCount   uint64
		AvgLatency   time.Duration
	})

	for _, client := range c.rpcPool.Clients {
		success, errors, latency := client.GetMetrics()
		metrics[client.URL] = struct {
			SuccessCount uint64
			ErrorCount   uint64
			AvgLatency   time.Duration
		}{
			SuccessCount: success,
			ErrorCount:   errors,
			AvgLatency:   latency,
		}
	}

	return metrics
}
