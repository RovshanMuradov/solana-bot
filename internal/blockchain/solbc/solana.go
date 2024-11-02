// internal/blockchain/solbc/solana.go
package solbc

import (
	"context"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"go.uber.org/zap"
)

type Blockchain struct {
	client *Client
	logger *zap.Logger
}

func NewBlockchain(client *Client, logger *zap.Logger) (*Blockchain, error) {
	if client == nil {
		return nil, fmt.Errorf("client cannot be nil")
	}

	return &Blockchain{
		client: client,
		logger: logger,
	}, nil
}

func (s *Blockchain) Name() string {
	return "Solana"
}

func (s *Blockchain) SendTransaction(ctx context.Context, tx interface{}) (string, error) {
	solTx, ok := tx.(*solana.Transaction)
	if !ok {
		return "", fmt.Errorf("invalid transaction type: expected *solana.Transaction")
	}

	signature, err := s.client.SendTransaction(ctx, solTx)
	if err != nil {
		s.logger.Error("Failed to send transaction",
			zap.Error(err),
			zap.String("tx_type", fmt.Sprintf("%T", tx)))
		return "", fmt.Errorf("failed to send transaction: %w", err)
	}

	return signature.String(), nil
}

func (s *Blockchain) GetRecentBlockhash(ctx context.Context) (solana.Hash, error) {
	hash, err := s.client.GetRecentBlockhash(ctx)
	if err != nil {
		s.logger.Error("Failed to get recent blockhash", zap.Error(err))
		return solana.Hash{}, fmt.Errorf("failed to get recent blockhash: %w", err)
	}
	return hash, nil
}

// SimulateTransaction simulates a transaction on the Solana blockchain
func (c *Client) SimulateTransaction(tx string) (string, error) {
	// Implement the method logic here
	return "", nil
}
