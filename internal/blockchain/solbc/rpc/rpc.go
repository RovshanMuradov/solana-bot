// internal/blockchain/solbc/rpc/rpc.go
package rpc

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gagliardetto/solana-go"
	solanarpc "github.com/gagliardetto/solana-go/rpc"
	"go.uber.org/zap"
)

// Основные константы
const (
	retryAttempts = 2
	retryDelay    = 500 * time.Millisecond
	reqTimeout    = 10 * time.Second
)

// Основные ошибки
var (
	ErrNoRPCNodes = fmt.Errorf("no RPC nodes available")
	ErrTimeout    = fmt.Errorf("request timeout")
)

// RPCClient представляет упрощенный RPC клиент
type RPCClient struct {
	nodes   []*solanarpc.Client
	urls    []string
	current int
	mu      sync.RWMutex
	logger  *zap.Logger
}

// NewClient создает новый RPC клиент
func NewClient(urls []string, logger *zap.Logger) (*RPCClient, error) {
	if len(urls) == 0 {
		return nil, ErrNoRPCNodes
	}

	nodes := make([]*solanarpc.Client, len(urls))
	for i, url := range urls {
		nodes[i] = solanarpc.New(url)
	}

	return &RPCClient{
		nodes:  nodes,
		urls:   urls,
		logger: logger.Named("rpc-client"),
	}, nil
}

// ExecuteWithRetry выполняет RPC-запрос с автоматическим переключением узлов при ошибке
func (c *RPCClient) ExecuteWithRetry(ctx context.Context, operation func(*solanarpc.Client) error) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, reqTimeout)
	defer cancel()

	c.mu.RLock()
	startIdx := c.current
	c.mu.RUnlock()

	for attempt := 0; attempt < retryAttempts; attempt++ {
		select {
		case <-timeoutCtx.Done():
			return ErrTimeout
		default:
			// Получаем текущий узел
			c.mu.Lock()
			node := c.nodes[c.current]
			url := c.urls[c.current]
			// Переключаемся на следующий узел для следующего запроса
			c.current = (c.current + 1) % len(c.nodes)
			c.mu.Unlock()

			// Выполняем операцию
			err := operation(node)
			if err == nil {
				return nil
			}

			c.logger.Debug("RPC request failed, trying next node",
				zap.String("url", url),
				zap.Error(err),
				zap.Int("attempt", attempt+1))

			// Если это не последняя попытка, делаем задержку
			if attempt < retryAttempts-1 {
				select {
				case <-timeoutCtx.Done():
					return ErrTimeout
				case <-time.After(retryDelay):
				}
			}

			// Если мы перепробовали все узлы, начинаем сначала
			if c.current == startIdx {
				c.logger.Warn("All RPC nodes failed, starting over",
					zap.Error(err))
			}
		}
	}

	return fmt.Errorf("all retry attempts failed")
}

// GetAccountInfo получает информацию об аккаунте
func (c *RPCClient) GetAccountInfo(ctx context.Context, pubkey solana.PublicKey) (*solanarpc.GetAccountInfoResult, error) {
	var result *solanarpc.GetAccountInfoResult
	err := c.ExecuteWithRetry(ctx, func(client *solanarpc.Client) error {
		var err error
		result, err = client.GetAccountInfoWithOpts(ctx, pubkey, &solanarpc.GetAccountInfoOpts{
			Encoding:   solana.EncodingBase64,
			Commitment: solanarpc.CommitmentConfirmed,
		})
		return err
	})
	return result, err
}

// GetLatestBlockhash получает последний blockhash
func (c *RPCClient) GetLatestBlockhash(ctx context.Context) (*solanarpc.GetLatestBlockhashResult, error) {
	var result *solanarpc.GetLatestBlockhashResult
	err := c.ExecuteWithRetry(ctx, func(client *solanarpc.Client) error {
		var err error
		result, err = client.GetLatestBlockhash(ctx, solanarpc.CommitmentFinalized)
		return err
	})
	return result, err
}

// SendTransaction отправляет транзакцию
func (c *RPCClient) SendTransaction(ctx context.Context, tx *solana.Transaction) (solana.Signature, error) {
	var signature solana.Signature
	err := c.ExecuteWithRetry(ctx, func(client *solanarpc.Client) error {
		var err error
		signature, err = client.SendTransactionWithOpts(ctx, tx, solanarpc.TransactionOpts{
			SkipPreflight:       true,
			PreflightCommitment: solanarpc.CommitmentFinalized,
		})
		return err
	})
	return signature, err
}

// Close закрывает клиент
func (c *RPCClient) Close() {}
