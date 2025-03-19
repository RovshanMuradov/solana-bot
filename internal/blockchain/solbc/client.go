// internal/blockchain/solbc/client.go
package solbc

import (
	"context"
	"fmt"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain"
	"go.uber.org/zap"
)

// Client – тонкий адаптер для взаимодействия с блокчейном Solana через solana-go.
type Client struct {
	rpc    *rpc.Client
	logger *zap.Logger
}

// NewClient создаёт новый клиент, принимая RPC URL и логгер через dependency injection.
func NewClient(rpcURL string, logger *zap.Logger) *Client {
	return &Client{
		rpc:    rpc.New(rpcURL),
		logger: logger.Named("solbc-client"),
	}
}

// GetRecentBlockhash получает последний blockhash с использованием стандартного метода solana-go.
func (c *Client) GetRecentBlockhash(ctx context.Context) (solana.Hash, error) {
	result, err := c.rpc.GetLatestBlockhash(ctx, rpc.CommitmentFinalized)
	if err != nil {
		c.logger.Error("GetRecentBlockhash error", zap.Error(err))
		return solana.Hash{}, err
	}
	return result.Value.Blockhash, nil
}

// SendTransaction отправляет транзакцию.
func (c *Client) SendTransaction(ctx context.Context, tx *solana.Transaction) (solana.Signature, error) {
	sig, err := c.rpc.SendTransaction(ctx, tx)
	if err != nil {
		c.logger.Error("SendTransaction error", zap.Error(err))
		return solana.Signature{}, err
	}
	return sig, nil
}

// GetAccountInfo получает информацию об аккаунте.
func (c *Client) GetAccountInfo(ctx context.Context, pubkey solana.PublicKey) (*rpc.GetAccountInfoResult, error) {
	result, err := c.rpc.GetAccountInfo(ctx, pubkey)
	if err != nil {
		c.logger.Debug("GetAccountInfo error", 
			zap.String("pubkey", pubkey.String()),
			zap.Error(err))
		return nil, err
	}
	return result, nil
}

// GetSignatureStatuses получает статусы транзакций.
func (c *Client) GetSignatureStatuses(ctx context.Context, signatures ...solana.Signature) (*rpc.GetSignatureStatusesResult, error) {
	result, err := c.rpc.GetSignatureStatuses(ctx, false, signatures...)
	if err != nil {
		c.logger.Error("GetSignatureStatuses error", zap.Error(err))
		return nil, err
	}
	return result, nil
}

// SendTransactionWithOpts отправляет транзакцию с заданными опциями.
func (c *Client) SendTransactionWithOpts(ctx context.Context, tx *solana.Transaction, opts blockchain.TransactionOptions) (solana.Signature, error) {
	sig, err := c.rpc.SendTransactionWithOpts(ctx, tx, rpc.TransactionOpts{
		SkipPreflight:       opts.SkipPreflight,
		PreflightCommitment: opts.PreflightCommitment,
	})
	if err != nil {
		c.logger.Error("SendTransactionWithOpts error", zap.Error(err))
		return solana.Signature{}, err
	}
	return sig, nil
}

// SimulateTransaction симулирует транзакцию и возвращает результат симуляции.
func (c *Client) SimulateTransaction(ctx context.Context, tx *solana.Transaction) (*blockchain.SimulationResult, error) {
	result, err := c.rpc.SimulateTransaction(ctx, tx)
	if err != nil {
		c.logger.Error("SimulateTransaction error", zap.Error(err))
		return nil, err
	}
	units := uint64(0)
	if result.Value.UnitsConsumed != nil {
		units = *result.Value.UnitsConsumed
	}
	return &blockchain.SimulationResult{
		Err:           result.Value.Err,
		Logs:          result.Value.Logs,
		UnitsConsumed: units,
	}, nil
}

// GetBalance получает баланс аккаунта.
func (c *Client) GetBalance(ctx context.Context, pubkey solana.PublicKey, commitment rpc.CommitmentType) (uint64, error) {
	result, err := c.rpc.GetBalance(ctx, pubkey, commitment)
	if err != nil {
		c.logger.Error("GetBalance error", zap.Error(err))
		return 0, err
	}
	return result.Value, nil
}

// WaitForTransactionConfirmation ожидает подтверждения транзакции (с простым polling‑механизмом).
func (c *Client) WaitForTransactionConfirmation(ctx context.Context, signature solana.Signature, _ rpc.CommitmentType) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	timeout := time.After(30 * time.Second)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return fmt.Errorf("confirmation timeout")
		case <-ticker.C:
			statuses, err := c.GetSignatureStatuses(ctx, signature)
			if err != nil {
				c.logger.Warn("Error getting signature statuses", zap.Error(err))
				continue
			}
			if statuses != nil && len(statuses.Value) > 0 && statuses.Value[0] != nil {
				status := statuses.Value[0]
				// Сравниваем с rpc.ConfirmationStatusFinalized и rpc.ConfirmationStatusConfirmed,
				// которые имеют тип rpc.ConfirmationStatusType.
				if status.ConfirmationStatus == rpc.ConfirmationStatusFinalized ||
					status.ConfirmationStatus == rpc.ConfirmationStatusConfirmed {
					return nil
				}
			}
		}
	}
}

// Гарантируем, что Client реализует интерфейс blockchain.Client.
var _ blockchain.Client = (*Client)(nil)
