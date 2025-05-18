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

// SendTransaction отправляет транзакцию c параметрами по умолчанию.
func (c *Client) SendTransaction(ctx context.Context, tx *solana.Transaction) (solana.Signature, error) {
	// Используем TransactionOpts с SkipPreflight=true для ускорения обработки транзакции
	opts := rpc.TransactionOpts{
		SkipPreflight:       true,
		PreflightCommitment: rpc.CommitmentProcessed,
	}

	sig, err := c.rpc.SendTransactionWithOpts(ctx, tx, opts)
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
	result, err := c.rpc.GetSignatureStatuses(ctx, true, signatures...)
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

// WaitForTransactionConfirmation ожидает подтверждения транзакции с возможностью указать уровень подтверждения.
// Таймаут и интервал между проверками
const (
	confirmationTimeout = 45 * time.Second
	checkInterval       = 200 * time.Millisecond
)

// Для каждого уровня commitment — набор допустимых confirmation statuses
var okStatuses = map[rpc.CommitmentType][]rpc.ConfirmationStatusType{
	rpc.CommitmentProcessed: {rpc.ConfirmationStatusProcessed, rpc.ConfirmationStatusConfirmed, rpc.ConfirmationStatusFinalized},
	rpc.CommitmentConfirmed: {rpc.ConfirmationStatusConfirmed, rpc.ConfirmationStatusFinalized},
	rpc.CommitmentFinalized: {rpc.ConfirmationStatusFinalized},
}

// contains проверяет, есть ли val в slice
func contains(slice []rpc.ConfirmationStatusType, val rpc.ConfirmationStatusType) bool {
	for _, v := range slice {
		if v == val {
			return true
		}
	}
	return false
}

// WaitForTransactionConfirmation ожидает нужного уровня подтверждения транзакции.
func (c *Client) WaitForTransactionConfirmation(
	ctx context.Context,
	signature solana.Signature,
	commitment rpc.CommitmentType,
) error {
	// По умолчанию — минимум Confirmed
	if commitment == "" {
		commitment = rpc.CommitmentConfirmed
	}

	// Обрезаем общий ctx таймаутом
	ctx, cancel := context.WithTimeout(ctx, confirmationTimeout)
	defer cancel()

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	c.logger.Info("Waiting for transaction confirmation",
		zap.String("signature", signature.String()),
		zap.String("commitment", string(commitment)),
	)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			resp, err := c.rpc.GetSignatureStatuses(ctx, true, signature)
			if err != nil {
				c.logger.Warn("Error getting signature statuses",
					zap.String("signature", signature.String()),
					zap.Error(err),
				)
				continue
			}
			if resp == nil || len(resp.Value) == 0 || resp.Value[0] == nil {
				continue
			}

			status := resp.Value[0]
			// Если транзакция упала — сразу возвращаем ошибку
			if status.Err != nil {
				return fmt.Errorf("transaction failed: %w", status.Err)
			}
			// Если дошли до нужного статуса — выходим
			if contains(okStatuses[commitment], status.ConfirmationStatus) {
				c.logger.Info("Transaction confirmed",
					zap.String("signature", signature.String()),
					zap.String("status", string(status.ConfirmationStatus)),
				)
				return nil
			}
		}
	}
}

// GetTokenAccountBalance получает баланс токенного аккаунта с указанным уровнем подтверждения.
func (c *Client) GetTokenAccountBalance(ctx context.Context, account solana.PublicKey, commitment rpc.CommitmentType) (*rpc.GetTokenAccountBalanceResult, error) {
	// Если commitment не указан, используем CommitmentConfirmed
	if commitment == "" {
		commitment = rpc.CommitmentConfirmed
	}

	result, err := c.rpc.GetTokenAccountBalance(ctx, account, commitment)
	if err != nil {
		c.logger.Debug("GetTokenAccountBalance error",
			zap.String("account", account.String()),
			zap.Error(err))
		return nil, err
	}
	return result, nil
}

// Гарантируем, что Client реализует интерфейс blockchain.Client.
var _ blockchain.Client = (*Client)(nil)
