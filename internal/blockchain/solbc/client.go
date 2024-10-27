// internal/blockchain/solbc/client.go
package solbc

import (
	"context"
	"fmt"
	"time"

	"github.com/gagliardetto/solana-go"
	solanarpc "github.com/gagliardetto/solana-go/rpc"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc/rpc"
	"go.uber.org/zap"
)

// NewClient создает новый экземпляр клиента с улучшенным мониторингом
func NewClient(rpcURLs []string, logger *zap.Logger) (*Client, error) {
	logger = logger.Named("solana-client")

	// Создаем улучшенный RPC клиент
	enhancedRPC, err := rpc.NewEnhancedClient(rpcURLs, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create enhanced RPC client: %w", err)
	}

	client := &Client{
		enhancedRPC: enhancedRPC,
		logger:      logger,
		metrics:     &ClientMetrics{},
	}

	// Запускаем периодическую проверку состояния
	go client.monitorHealth()

	return client, nil
}

// monitorHealth периодически проверяет состояние RPC подключений
func (c *Client) monitorHealth() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		metrics := c.enhancedRPC.GetMetrics()
		c.logger.Info("RPC health status",
			zap.Int32("active_nodes", metrics.ActiveNodes),
			zap.Uint64("total_requests", metrics.TotalRequests),
			zap.Uint64("failed_requests", metrics.FailedRequests),
			zap.Duration("avg_latency", metrics.AverageLatency),
			zap.Time("last_successful", metrics.LastSuccessful))
	}
}

// // validateConnections проверяет подключения ко всем RPC узлам
// func (c *Client) validateConnections(ctx context.Context) error {
// 	ctx, cancel := context.WithTimeout(ctx, rpc.DefaultTimeout)
// 	defer cancel()

// 	var wg sync.WaitGroup
// 	errChan := make(chan error, len(c.rpcPool.Clients))

// 	for _, client := range c.rpcPool.Clients {
// 		wg.Add(1)
// 		go func(node *rpc.NodeClient) {
// 			defer wg.Done()

// 			var lastErr error
// 			for attempt := 0; attempt < rpc.MaxRetries; attempt++ {
// 				start := time.Now()
// 				if err := c.testConnection(ctx, node); err != nil {
// 					lastErr = err
// 					node.UpdateMetrics(false, time.Since(start))
// 					time.Sleep(rpc.RetryDelay)
// 					continue
// 				}
// 				node.UpdateMetrics(true, time.Since(start))
// 				return
// 			}
// 			if lastErr != nil {
// 				errChan <- rpc.NewError(lastErr, node.URL, "validate_connection")
// 				node.SetActive(false)
// 			}
// 		}(client)
// 	}

// 	wg.Wait()
// 	close(errChan)

// 	// Собираем все ошибки
// 	var errors []error
// 	for err := range errChan {
// 		errors = append(errors, err)
// 	}

// 	// Проверяем наличие активных клиентов
// 	if !c.rpcPool.HasActiveClients() {
// 		return fmt.Errorf("no active RPC connections available: %v", errors)
// 	}

// 	if len(errors) > 0 {
// 		c.logger.Warn("Some RPC connections failed",
// 			zap.Int("total", len(c.rpcPool.Clients)),
// 			zap.Int("failed", len(errors)))
// 	}

// 	return nil
// }

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
// GetAccountInfo получает информацию об аккаунте с расширенной диагностикой
func (c *Client) GetAccountInfo(ctx context.Context, pubkey solana.PublicKey) (*solanarpc.GetAccountInfoResult, error) {
	c.logger.Debug("Getting account info",
		zap.String("pubkey", pubkey.String()),
		zap.Time("request_time", time.Now()))

	var result *solanarpc.GetAccountInfoResult
	var lastErr error

	err := c.enhancedRPC.ExecuteWithRetry(ctx, func(node *rpc.NodeClient) error {
		start := time.Now()
		var err error
		result, err = node.Client.GetAccountInfoWithOpts(ctx, pubkey, &solanarpc.GetAccountInfoOpts{
			Encoding:   solana.EncodingBase64,
			Commitment: solanarpc.CommitmentConfirmed,
		})

		duration := time.Since(start)
		if err != nil {
			lastErr = err
			c.logger.Warn("Failed to get account info from node",
				zap.String("node_url", node.URL),
				zap.Duration("duration", duration),
				zap.Error(err))
			return err
		}

		c.logger.Debug("Successfully got account info",
			zap.String("node_url", node.URL),
			zap.Duration("duration", duration))
		return nil
	})

	if err != nil {
		c.metrics.FailedRequests++
		c.metrics.LastError = lastErr
		c.metrics.LastErrorTime = time.Now()

		return nil, fmt.Errorf("failed to get account info after retries: %w (last error: %v)",
			err, lastErr)
	}

	c.metrics.AccountInfoRequests++
	return result, nil
}

// GetRecentBlockhash получает последний блокхеш
// GetRecentBlockhash получает последний блокхеш с повторными попытками
func (c *Client) GetRecentBlockhash(ctx context.Context) (solana.Hash, error) {
	var hash solana.Hash
	var lastErr error

	err := c.enhancedRPC.ExecuteWithRetry(ctx, func(node *rpc.NodeClient) error {
		start := time.Now()
		result, err := node.Client.GetLatestBlockhash(ctx, solanarpc.CommitmentFinalized)
		duration := time.Since(start)

		if err != nil {
			lastErr = err
			c.logger.Warn("Failed to get recent blockhash",
				zap.String("node_url", node.URL),
				zap.Duration("duration", duration),
				zap.Error(err))
			return err
		}

		hash = result.Value.Blockhash
		c.logger.Debug("Successfully got recent blockhash",
			zap.String("node_url", node.URL),
			zap.Duration("duration", duration))
		return nil
	})

	if err != nil {
		return solana.Hash{}, fmt.Errorf("failed to get recent blockhash: %w (last error: %v)",
			err, lastErr)
	}

	return hash, nil
}

// SendTransaction отправляет транзакцию
// SendTransaction отправляет транзакцию с улучшенной обработкой ошибок
func (c *Client) SendTransaction(ctx context.Context, tx *solana.Transaction) (solana.Signature, error) {
	var signature solana.Signature
	var lastErr error

	err := c.enhancedRPC.ExecuteWithRetry(ctx, func(node *rpc.NodeClient) error {
		start := time.Now()
		sig, err := node.Client.SendTransactionWithOpts(ctx, tx, solanarpc.TransactionOpts{
			SkipPreflight:       true,
			PreflightCommitment: solanarpc.CommitmentFinalized,
		})
		duration := time.Since(start)

		if err != nil {
			lastErr = err
			c.logger.Warn("Failed to send transaction",
				zap.String("node_url", node.URL),
				zap.Duration("duration", duration),
				zap.Error(err))
			return err
		}

		signature = sig
		c.logger.Debug("Successfully sent transaction",
			zap.String("node_url", node.URL),
			zap.String("signature", sig.String()),
			zap.Duration("duration", duration))
		return nil
	})

	if err != nil {
		c.metrics.FailedRequests++
		return solana.Signature{}, fmt.Errorf("failed to send transaction: %w (last error: %v)",
			err, lastErr)
	}

	c.metrics.TransactionRequests++
	return signature, nil
}

// GetMetrics возвращает текущие метрики клиента
func (c *Client) GetMetrics() map[string]interface{} {
	rpcMetrics := c.enhancedRPC.GetMetrics()

	return map[string]interface{}{
		"rpc_metrics": map[string]interface{}{
			"active_nodes":    rpcMetrics.ActiveNodes,
			"total_requests":  rpcMetrics.TotalRequests,
			"failed_requests": rpcMetrics.FailedRequests,
			"average_latency": rpcMetrics.AverageLatency,
			"last_successful": rpcMetrics.LastSuccessful,
		},
		"client_metrics": map[string]interface{}{
			"account_info_requests": c.metrics.AccountInfoRequests,
			"transaction_requests":  c.metrics.TransactionRequests,
			"failed_requests":       c.metrics.FailedRequests,
			"last_error_time":       c.metrics.LastErrorTime,
		},
	}
}

func (c *Client) Close() error {
	c.enhancedRPC.Close()
	return nil
}
