package solana

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"go.uber.org/zap"
)

// NewClient создает новый экземпляр клиента Solana
func NewClient(rpcURLs []string, logger *zap.Logger) (*Client, error) {
	if len(rpcURLs) == 0 {
		return nil, errors.New("empty RPC URL list")
	}

	var clients []*RPCClient
	for _, urlStr := range rpcURLs {
		if _, err := url.Parse(urlStr); err != nil {
			logger.Warn("Invalid RPC URL", zap.String("url", urlStr), zap.Error(err))
			continue
		}

		client := &RPCClient{
			Client:  rpc.New(urlStr),
			URL:     urlStr,
			active:  true,
			metrics: &RPCMetrics{},
		}
		clients = append(clients, client)
	}

	if len(clients) == 0 {
		return nil, errors.New("no valid RPC URLs provided")
	}

	c := &Client{
		rpcClients: clients,
		logger:     logger,
	}

	if err := c.validateConnections(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to validate connections: %w", err)
	}

	return c, nil
}

// testConnection проверяет подключение к RPC узлу
func (c *Client) testConnection(ctx context.Context, rpcClient *RPCClient) error {
	// Пробуем получить версию узла как более легкий запрос
	version, err := rpcClient.Client.GetVersion(ctx)
	if err != nil {
		return fmt.Errorf("failed to get version: %w", err)
	}

	// Если версия получена успешно, пробуем получить последний блокхеш
	_, err = rpcClient.Client.GetLatestBlockhash(ctx, rpc.CommitmentFinalized)
	if err != nil {
		return fmt.Errorf("failed to get latest blockhash: %w", err)
	}

	c.logger.Debug("Successfully connected to RPC",
		zap.String("url", rpcClient.URL),
		zap.String("solana_core", version.SolanaCore))

	return nil
}

func (c *Client) validateConnections(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var wg sync.WaitGroup
	errChan := make(chan error, len(c.rpcClients))

	for _, client := range c.rpcClients {
		wg.Add(1)
		go func(rpcClient *RPCClient) {
			defer wg.Done()

			var lastErr error
			for attempt := 0; attempt < maxRetries; attempt++ {
				start := time.Now()
				if err := c.testConnection(ctx, rpcClient); err != nil {
					lastErr = err
					rpcClient.updateMetrics(false, time.Since(start))
					time.Sleep(retryDelay)
					continue
				}
				rpcClient.updateMetrics(true, time.Since(start))
				return
			}
			if lastErr != nil {
				errChan <- fmt.Errorf("failed to connect to %s: %w", rpcClient.URL, lastErr)
				rpcClient.setActive(false)
			}
		}(client)
	}

	wg.Wait()
	close(errChan)

	// Проверяем наличие активных клиентов
	if !c.hasActiveClients() {
		return errors.New("no active RPC connections available")
	}

	return nil
}

// GetAccountInfo получает информацию об аккаунте
func (c *Client) GetAccountInfo(
	ctx context.Context,
	pubkey solana.PublicKey,
) (*rpc.GetAccountInfoResult, error) {
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		client := c.getNextClient()
		if client == nil {
			return nil, errors.New("no active RPC clients available")
		}

		start := time.Now()
		result, err := client.Client.GetAccountInfoWithOpts(ctx, pubkey, &rpc.GetAccountInfoOpts{
			Encoding:   solana.EncodingBase64,
			Commitment: rpc.CommitmentConfirmed,
		})
		client.updateMetrics(err == nil, time.Since(start))

		if err != nil {
			lastErr = err
			client.setActive(false)
			continue
		}

		return result, nil
	}

	return nil, fmt.Errorf("failed to get account info after %d attempts: %w", maxRetries, lastErr)
}

func (c *Client) GetRecentBlockhash(ctx context.Context) (solana.Hash, error) {
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		client := c.getNextClient()
		if client == nil {
			return solana.Hash{}, errors.New("no active RPC clients available")
		}

		start := time.Now()
		result, err := client.Client.GetLatestBlockhash(ctx, rpc.CommitmentFinalized)
		client.updateMetrics(err == nil, time.Since(start))

		if err != nil {
			lastErr = err
			client.setActive(false)
			continue
		}

		return result.Value.Blockhash, nil
	}

	return solana.Hash{}, fmt.Errorf("failed to get recent blockhash after %d attempts: %w", maxRetries, lastErr)
}

func (c *Client) SendTransaction(ctx context.Context, tx *solana.Transaction) (solana.Signature, error) {
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		client := c.getNextClient()
		if client == nil {
			return solana.Signature{}, errors.New("no active RPC clients available")
		}

		start := time.Now()
		sig, err := client.Client.SendTransactionWithOpts(ctx, tx, rpc.TransactionOpts{
			SkipPreflight:       true,
			PreflightCommitment: rpc.CommitmentFinalized,
		})
		client.updateMetrics(err == nil, time.Since(start))

		if err != nil {
			lastErr = err
			client.setActive(false)
			continue
		}

		return sig, nil
	}

	return solana.Signature{}, fmt.Errorf("failed to send transaction after %d attempts: %w", maxRetries, lastErr)
}

// Вспомогательные методы для Client
func (c *Client) hasActiveClients() bool {
	for _, client := range c.rpcClients {
		if client.isActive() {
			return true
		}
	}
	return false
}

func (c *Client) getNextClient() *RPCClient {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	initialIndex := c.currIndex
	for {
		c.currIndex = (c.currIndex + 1) % len(c.rpcClients)
		if c.rpcClients[c.currIndex].isActive() {
			return c.rpcClients[c.currIndex]
		}
		if c.currIndex == initialIndex {
			return nil
		}
	}
}
