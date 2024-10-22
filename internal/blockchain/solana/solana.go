package solana

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
		return "", fmt.Errorf("invalid transaction type for Solana")
	}

	signature, err := s.client.SendTransaction(ctx, solTx)
	if err != nil {
		return "", err
	}
	return signature.String(), nil
}

func (s *Blockchain) GetRecentBlockhash(ctx context.Context) (solana.Hash, error) {
	return s.client.GetRecentBlockhash(ctx)
}
