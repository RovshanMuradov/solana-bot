// internal/blockchain/solbc/client.go
package solbc

import (
	"context"
	"fmt"
	"time"

	"github.com/gagliardetto/solana-go"
	solanarpc "github.com/gagliardetto/solana-go/rpc"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc/rpc"
	"go.uber.org/zap"
)

// NewClient создает новый экземпляр клиента с улучшенным мониторингом
func NewClient(rpcURLs []string, privateKey solana.PrivateKey, logger *zap.Logger) (*Client, error) {
	logger = logger.Named("solana-client")

	rpcClient, err := rpc.NewClient(rpcURLs, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create RPC client: %w", err)
	}

	return &Client{
		rpc:        rpcClient,
		logger:     logger,
		metrics:    &ClientMetrics{},
		privateKey: privateKey,
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

// GetProgramAccounts получает аккаунты программы по заданным фильтрам
func (c *Client) GetProgramAccounts(
	ctx context.Context,
	program solana.PublicKey,
	opts solanarpc.GetProgramAccountsOpts,
) ([]solanarpc.KeyedAccount, error) {
	c.logger.Debug("getting program accounts",
		zap.String("program", program.String()),
	)

	accounts, err := c.rpc.GetProgramAccounts(ctx, program, opts)
	if err != nil {
		c.metrics.IncrementFailedRequests()
		c.metrics.LastError = err
		c.metrics.LastErrorTime = time.Now()
		return nil, fmt.Errorf("failed to get program accounts: %w", err)
	}

	c.metrics.IncrementProgramAccountRequests()
	return accounts, nil
}

// GetTokenAccountBalance получает баланс токен-аккаунта
func (c *Client) GetTokenAccountBalance(
	ctx context.Context,
	account solana.PublicKey,
	commitment solanarpc.CommitmentType,
) (*solanarpc.GetTokenAccountBalanceResult, error) {
	c.logger.Debug("getting token account balance",
		zap.String("account", account.String()),
		zap.String("commitment", string(commitment)),
	)

	result, err := c.rpc.GetTokenAccountBalance(ctx, account, commitment)
	if err != nil {
		c.metrics.FailedRequests++
		c.metrics.LastError = err
		c.metrics.LastErrorTime = time.Now()
		return nil, fmt.Errorf("failed to get token account balance: %w", err)
	}

	return result, nil
}

// SimulateTransaction симулирует выполнение транзакции
func (c *Client) SimulateTransaction(
	ctx context.Context,
	tx *solana.Transaction,
) (*blockchain.SimulationResult, error) {
	c.logger.Debug("simulating transaction")

	result, err := c.rpc.SimulateTransaction(ctx, tx)
	if err != nil {
		c.metrics.FailedRequests++
		c.metrics.LastError = err
		c.metrics.LastErrorTime = time.Now()
		return nil, fmt.Errorf("failed to simulate transaction: %w", err)
	}

	// Обработаем возможный nil в UnitsConsumed
	var unitsConsumed uint64
	if result.Value.UnitsConsumed != nil {
		unitsConsumed = *result.Value.UnitsConsumed
	}

	// Преобразуем результат в нужный формат
	simulationResult := &blockchain.SimulationResult{
		Err:           result.Value.Err,
		Logs:          result.Value.Logs,
		UnitsConsumed: unitsConsumed,
	}

	return simulationResult, nil
}

// TODO: Этот метод реализован в sdk solana-go, надо переписать
// GetBalance реализует интерфейс blockchain.Client
func (c *Client) GetBalance(
	ctx context.Context,
	pubkey solana.PublicKey,
	commitment solanarpc.CommitmentType,
) (uint64, error) {
	c.logger.Debug("getting balance",
		zap.String("pubkey", pubkey.String()),
		zap.String("commitment", string(commitment)),
	)

	result, err := c.rpc.GetBalance(ctx, pubkey, commitment)
	if err != nil {
		c.metrics.FailedRequests++
		c.metrics.LastError = err
		c.metrics.LastErrorTime = time.Now()
		return 0, fmt.Errorf("failed to get balance: %w", err)
	}

	c.metrics.BalanceRequests++
	return result.Value, nil
}

// GetRPCEndpoint возвращает текущий активный RPC endpoint
func (c *Client) GetRPCEndpoint() string {
	if c.rpc == nil {
		return ""
	}

	// Получаем текущий индекс и URLs из RPC клиента
	// Добавляем метод для получения текущего URL в RPCClient
	return c.rpc.GetCurrentURL()
}

// GetWalletKey возвращает приватный ключ кошелька
func (c *Client) GetWalletKey() (solana.PrivateKey, error) {
	if c.privateKey == nil {
		return nil, fmt.Errorf("private key not set")
	}
	return c.privateKey, nil
}
func (c *Client) WaitForTransactionConfirmation(ctx context.Context, signature solana.Signature, commitment solanarpc.CommitmentType) error {
	c.logger.Debug("waiting for transaction confirmation",
		zap.String("signature", signature.String()),
		zap.String("commitment", string(commitment)))

	for i := 0; i < 30; i++ {
		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled while waiting for confirmation: %w", ctx.Err())
		default:
			status, err := c.GetSignatureStatuses(ctx, signature)
			if err != nil {
				c.metrics.FailedRequests++
				return fmt.Errorf("failed to get signature status: %w", err)
			}

			if status != nil && len(status.Value) > 0 && status.Value[0] != nil {
				if status.Value[0].Err != nil {
					c.metrics.FailedRequests++
					return fmt.Errorf("transaction failed: %v", status.Value[0].Err)
				}

				confirmStatus := string(status.Value[0].ConfirmationStatus)
				switch {
				case commitment == solanarpc.CommitmentConfirmed &&
					(confirmStatus == string(solanarpc.CommitmentConfirmed) || confirmStatus == string(solanarpc.CommitmentFinalized)):
					c.metrics.SuccessfulConfirmations++
					return nil
				case commitment == solanarpc.CommitmentFinalized &&
					confirmStatus == string(solanarpc.CommitmentFinalized):
					c.metrics.SuccessfulConfirmations++
					return nil
				}
			}

			time.Sleep(500 * time.Millisecond)
		}
	}

	c.metrics.FailedRequests++
	return fmt.Errorf("confirmation timeout after 30 attempts")
}
