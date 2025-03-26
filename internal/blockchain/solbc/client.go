// internal/blockchain/solbc/client.go
package solbc

import (
	"context"
	"errors"
	"fmt"
	"strings"
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

// Определение ошибок
var (
	ErrAccountNotFound = errors.New("account not found")
)

// IsAccountNotFoundError проверяет, является ли ошибка "not found"
func IsAccountNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "not found")
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

// GetAccountDataInto получает данные аккаунта и декодирует их в указанную структуру.
func (c *Client) GetAccountDataInto(ctx context.Context, pubkey solana.PublicKey, dst interface{}) error {
	err := c.rpc.GetAccountDataInto(ctx, pubkey, dst)
	if err != nil {
		c.logger.Debug("GetAccountDataInto error",
			zap.String("pubkey", pubkey.String()),
			zap.Error(err))
		return err
	}
	return nil
}

// // GetAccountInfo получает информацию об аккаунте.
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

// GetMultipleAccounts получает информацию о нескольких аккаунтах за один запрос
func (c *Client) GetMultipleAccounts(
	ctx context.Context,
	pubkeys []solana.PublicKey,
) (*rpc.GetMultipleAccountsResult, error) {
	if len(pubkeys) == 0 {
		return &rpc.GetMultipleAccountsResult{}, nil
	}

	// Создаем опции запроса
	opts := rpc.GetMultipleAccountsOpts{
		Commitment: rpc.CommitmentConfirmed,
		Encoding:   solana.EncodingBase64,
	}

	// Выполняем запрос
	res, err := c.rpc.GetMultipleAccountsWithOpts(
		ctx,
		pubkeys, // Передаем непосредственно []solana.PublicKey
		&opts,
	)

	if err != nil {
		c.logger.Debug("GetMultipleAccounts error",
			zap.Error(err))
		return nil, err
	}

	return res, nil
}

// GetProgramAccounts получает все аккаунты программы с фильтрами
func (c *Client) GetProgramAccounts(
	ctx context.Context,
	programID solana.PublicKey,
	discriminator []byte,
) (*rpc.GetProgramAccountsResult, error) {
	// Создаем опции запроса с фильтрами
	opts := rpc.GetProgramAccountsOpts{
		Commitment: rpc.CommitmentConfirmed,
		Encoding:   solana.EncodingBase64,
	}

	// Добавляем фильтр по дискриминатору, если он предоставлен
	if len(discriminator) > 0 {
		// Используем DataSlice для фильтрации по первым байтам (дискриминатору)
		offset := uint64(0)
		length := uint64(len(discriminator))
		opts.DataSlice = &rpc.DataSlice{
			Offset: &offset,
			Length: &length,
		}

		// Используем Filters для мемкомпа
		opts.Filters = append(opts.Filters,
			rpc.RPCFilter{
				Memcmp: &rpc.RPCFilterMemcmp{
					Offset: 0,
					Bytes:  discriminator,
				},
			},
		)
	}

	// Выполняем запрос через RPC клиент
	accounts, err := c.rpc.GetProgramAccountsWithOpts(
		ctx,
		programID,
		&opts,
	)

	if err != nil {
		c.logger.Debug("GetProgramAccounts error",
			zap.String("program_id", programID.String()),
			zap.Error(err))
		return nil, err
	}

	return &accounts, nil
}

// GetProgramAccountsWithOpts получает все аккаунты программы с опциями фильтрации
func (c *Client) GetProgramAccountsWithOpts(
	ctx context.Context,
	programID solana.PublicKey,
	opts *rpc.GetProgramAccountsOpts,
) (rpc.GetProgramAccountsResult, error) {
	accounts, err := c.rpc.GetProgramAccountsWithOpts(ctx, programID, opts)
	if err != nil {
		c.logger.Debug("GetProgramAccountsWithOpts error",
			zap.String("program_id", programID.String()),
			zap.Error(err))
		return nil, err
	}
	return accounts, nil
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

// GetTokenAccountBalance получает баланс токенного аккаунта
func (c *Client) GetTokenAccountBalance(ctx context.Context, account solana.PublicKey) (*rpc.GetTokenAccountBalanceResult, error) {
	// Передаем rpc.CommitmentConfirmed в качестве уровня комитмента
	return c.rpc.GetTokenAccountBalance(ctx, account, rpc.CommitmentConfirmed)
}

// Гарантируем, что Client реализует интерфейс blockchain.Client.
var _ blockchain.Client = (*Client)(nil)
