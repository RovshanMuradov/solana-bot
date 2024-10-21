package solana

import (
	"context"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"go.uber.org/zap"
)

type SolanaBlockchain struct {
	client *Client
	logger *zap.Logger
}

func NewSolanaBlockchain(client *Client, logger *zap.Logger) (*SolanaBlockchain, error) {
	return &SolanaBlockchain{
		client: client,
		logger: logger,
	}, nil
}

func (s *SolanaBlockchain) Name() string {
	return "Solana"
}

func (s *SolanaBlockchain) SendTransaction(ctx context.Context, tx interface{}) (string, error) {
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

func (s *SolanaBlockchain) GetRecentBlockhash(ctx context.Context) (solana.Hash, error) {
	return s.client.GetRecentBlockhash(ctx)
}
