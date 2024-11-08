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

// SendTransactionOpts определяет опции для отправки транзакции
type SendTransactionOpts struct {
	SkipPreflight       bool
	PreflightCommitment solanarpc.CommitmentType
}

// RPCClient представляет упрощенный RPC клиент
type Client struct {
	nodes   []*solanarpc.Client
	urls    []string
	current int
	mu      sync.RWMutex
	logger  *zap.Logger
}

// NewClient создает новый RPC клиент
func NewClient(urls []string, logger *zap.Logger) (*Client, error) {
	if len(urls) == 0 {
		return nil, ErrNoRPCNodes
	}

	nodes := make([]*solanarpc.Client, len(urls))
	for i, url := range urls {
		nodes[i] = solanarpc.New(url)
	}

	return &Client{
		nodes:  nodes,
		urls:   urls,
		logger: logger.Named("rpc-client"),
	}, nil
}

// SendTransactionWithOpts отправляет транзакцию с опциями
func (c *Client) SendTransactionWithOpts(
	ctx context.Context,
	tx *solana.Transaction,
	opts SendTransactionOpts,
) (solana.Signature, error) {
	var signature solana.Signature
	err := c.ExecuteWithRetry(ctx, func(client *solanarpc.Client) error {
		var err error
		signature, err = client.SendTransactionWithOpts(ctx, tx, solanarpc.TransactionOpts{
			SkipPreflight:       opts.SkipPreflight,
			PreflightCommitment: opts.PreflightCommitment,
		})
		return err
	})
	if err != nil {
		return solana.Signature{}, fmt.Errorf("failed to send transaction: %w", err)
	}
	return signature, nil
}

// ExecuteWithRetry выполняет RPC-запрос с автоматическим переключением узлов при ошибке
func (c *Client) ExecuteWithRetry(ctx context.Context, operation func(*solanarpc.Client) error) error {
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
func (c *Client) GetAccountInfo(ctx context.Context, pubkey solana.PublicKey) (*solanarpc.GetAccountInfoResult, error) {
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
func (c *Client) GetLatestBlockhash(ctx context.Context) (*solanarpc.GetLatestBlockhashResult, error) {
	var result *solanarpc.GetLatestBlockhashResult
	err := c.ExecuteWithRetry(ctx, func(client *solanarpc.Client) error {
		var err error
		result, err = client.GetLatestBlockhash(ctx, solanarpc.CommitmentFinalized)
		return err
	})
	return result, err
}

// SendTransaction отправляет транзакцию
func (c *Client) SendTransaction(ctx context.Context, tx *solana.Transaction) (solana.Signature, error) {
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

// Добавляем новый метод в RPCClient
func (c *Client) GetSignatureStatuses(ctx context.Context, signatures ...solana.Signature) (*solanarpc.GetSignatureStatusesResult, error) {
	var result *solanarpc.GetSignatureStatusesResult
	err := c.ExecuteWithRetry(ctx, func(client *solanarpc.Client) error {
		var err error
		result, err = client.GetSignatureStatuses(ctx, false, signatures...)
		return err
	})
	return result, err
}

// Close закрывает клиент
func (c *Client) Close() {}

// GetProgramAccounts получает все аккаунты для заданной программы
func (c *Client) GetProgramAccounts(
	ctx context.Context,
	program solana.PublicKey,
	opts solanarpc.GetProgramAccountsOpts,
) ([]solanarpc.KeyedAccount, error) {
	var accounts []solanarpc.KeyedAccount

	err := c.ExecuteWithRetry(ctx, func(client *solanarpc.Client) error {
		result, err := client.GetProgramAccountsWithOpts(
			ctx,
			program,
			&opts,
		)
		if err != nil {
			return err
		}

		// Преобразуем []*KeyedAccount в []KeyedAccount
		accounts = make([]solanarpc.KeyedAccount, len(result))
		for i, acc := range result {
			accounts[i] = *acc
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get program accounts: %w", err)
	}

	return accounts, nil
}

// GetTokenAccountBalance получает баланс токен-аккаунта
func (c *Client) GetTokenAccountBalance(
	ctx context.Context,
	account solana.PublicKey,
	commitment solanarpc.CommitmentType,
) (*solanarpc.GetTokenAccountBalanceResult, error) {
	var result *solanarpc.GetTokenAccountBalanceResult

	err := c.ExecuteWithRetry(ctx, func(client *solanarpc.Client) error {
		var err error
		result, err = client.GetTokenAccountBalance(
			ctx,
			account,
			commitment,
		)
		return err
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get token account balance: %w", err)
	}

	return result, nil
}

// SimulateTransaction симулирует выполнение транзакции
func (c *Client) SimulateTransaction(
	ctx context.Context,
	tx *solana.Transaction,
) (*solanarpc.SimulateTransactionResponse, error) {
	var result *solanarpc.SimulateTransactionResponse

	err := c.ExecuteWithRetry(ctx, func(client *solanarpc.Client) error {
		var err error
		result, err = client.SimulateTransactionWithOpts(ctx, tx, &solanarpc.SimulateTransactionOpts{
			SigVerify:              false,
			Commitment:             solanarpc.CommitmentConfirmed,
			ReplaceRecentBlockhash: false,
		})
		return err
	})

	if err != nil {
		return nil, fmt.Errorf("failed to simulate transaction: %w", err)
	}

	return result, nil
}

func (c *Client) GetBalance(
	ctx context.Context,
	pubkey solana.PublicKey,
	commitment solanarpc.CommitmentType,
) (*solanarpc.GetBalanceResult, error) {
	var result *solanarpc.GetBalanceResult

	err := c.ExecuteWithRetry(ctx, func(client *solanarpc.Client) error {
		var err error
		result, err = client.GetBalance(ctx, pubkey, commitment)
		if err != nil {
			return fmt.Errorf("RPC GetBalance failed: %w", err)
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get balance after retries: %w", err)
	}

	return result, nil
}

// GetCurrentURL возвращает текущий активный URL
func (c *Client) GetCurrentURL() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.urls) == 0 {
		return ""
	}

	// Возвращаем текущий активный URL
	return c.urls[c.current]
}
