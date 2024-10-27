// internal/blockchain/solbc/transaction/monitor.go
package transaction

import (
	"context"
	"fmt"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"go.uber.org/zap"
)

type Monitor struct {
	client  *rpc.Client
	logger  *zap.Logger
	config  Config
	metrics *Metrics
}

func NewMonitor(client *rpc.Client, logger *zap.Logger, config Config) *Monitor {
	if config.MinConfirmations == 0 {
		config.MinConfirmations = 1
	}
	return &Monitor{
		client:  client,
		logger:  logger.Named("tx-monitor"),
		config:  config,
		metrics: NewMetrics(),
	}
}

// checkConfirmation проверяет, подтверждена ли транзакция
func (m *Monitor) checkConfirmation(ctx context.Context, signature solana.Signature) (bool, error) {
	response, err := m.client.GetSignatureStatuses(ctx, false, signature)
	if err != nil {
		return false, fmt.Errorf("failed to get signature status: %w", err)
	}

	if len(response.Value) == 0 || response.Value[0] == nil {
		return false, nil
	}

	status := response.Value[0]

	if status.Confirmations != nil && *status.Confirmations >= uint64(m.config.MinConfirmations) {
		return true, nil
	}

	return status.ConfirmationStatus == rpc.ConfirmationStatusFinalized, nil
}

func (m *Monitor) GetTransactionStatus(ctx context.Context, signature solana.Signature) (*Status, error) {
	response, err := m.client.GetSignatureStatuses(ctx, false, signature)
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction status: %w", err)
	}

	if response == nil || len(response.Value) == 0 || response.Value[0] == nil {
		return &Status{
			Signature: signature.String(),
			Status:    "pending",
			Timestamp: time.Now(),
		}, nil
	}

	status := response.Value[0]
	txStatus := &Status{
		Signature: signature.String(),
		Timestamp: time.Now(),
		Slot:      status.Slot,
	}

	if status.Confirmations != nil {
		txStatus.Confirmations = *status.Confirmations
	}

	switch status.ConfirmationStatus {
	case rpc.ConfirmationStatusFinalized:
		txStatus.Status = "finalized"
	case rpc.ConfirmationStatusConfirmed:
		txStatus.Status = "confirmed"
	default:
		txStatus.Status = "pending"
	}

	if status.Err != nil {
		errMsg := fmt.Sprintf("%v", status.Err)
		txStatus.Error = errMsg
		txStatus.Status = "failed"
	}

	return txStatus, nil
}

func (m *Monitor) AwaitConfirmation(ctx context.Context, signature solana.Signature) (*Status, error) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	deadline := time.After(m.config.ConfirmationTime)

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-deadline:
			return nil, ErrConfirmationTimeout
		case <-ticker.C:
			// Сначала проверяем базовое подтверждение
			confirmed, err := m.checkConfirmation(ctx, signature)
			if err != nil {
				m.logger.Warn("Confirmation check failed", zap.Error(err))
				continue
			}

			// Если транзакция подтверждена, получаем полный статус
			if confirmed {
				return m.GetTransactionStatus(ctx, signature)
			}
		}
	}
}
