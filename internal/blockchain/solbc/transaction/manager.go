// internal/blockchain/solbc/transaction/manager.go
package transaction

import (
	"context"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"go.uber.org/zap"
)

type Manager struct {
	client    *rpc.Client
	logger    *zap.Logger
	config    Config
	validator *Validator
	monitor   *Monitor
	metrics   *Metrics
}

func NewManager(client *rpc.Client, logger *zap.Logger, config Config) *Manager {
	return &Manager{
		client:    client,
		logger:    logger.Named("tx-manager"),
		config:    config,
		validator: NewValidator(logger),
		monitor:   NewMonitor(client, logger, config),
		metrics:   NewMetrics(),
	}
}

func (tm *Manager) SendAndConfirm(ctx context.Context, tx *solana.Transaction) (*Status, error) {
	defer tm.metrics.TrackTransaction(time.Now())

	if err := tm.validator.ValidateTransaction(tx); err != nil {
		tm.logger.Error("Transaction validation failed", zap.Error(err))
		return nil, err
	}

	signature, err := tm.sendWithRetry(ctx, tx)
	if err != nil {
		tm.logger.Error("Failed to send transaction", zap.Error(err))
		return nil, err
	}

	status, err := tm.monitor.AwaitConfirmation(ctx, signature)
	if err != nil {
		tm.logger.Error("Transaction confirmation failed",
			zap.String("signature", signature.String()),
			zap.Error(err))
		return nil, err
	}

	return status, nil
}

func (tm *Manager) sendWithRetry(ctx context.Context, tx *solana.Transaction) (solana.Signature, error) {
	var signature solana.Signature
	operation := func() error {
		var err error
		signature, err = tm.client.SendTransactionWithOpts(ctx, tx, rpc.TransactionOpts{
			SkipPreflight:       tm.config.SkipPreflight,
			PreflightCommitment: tm.config.Commitment,
		})
		if err != nil {
			tm.metrics.failureCounter.Inc()
			tm.logger.Warn("Retrying transaction send", zap.Error(err))
			return err
		}
		tm.metrics.successCounter.Inc()
		return nil
	}

	err := backoff.Retry(operation, backoff.WithContext(backoff.NewExponentialBackOff(), ctx))
	if err != nil {
		return solana.Signature{}, err
	}
	return signature, nil
}
