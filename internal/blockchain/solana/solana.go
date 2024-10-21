// internal/blockchain/solana/solana.go
package solana

import (
	"context"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"go.uber.org/zap"
)

type SolanaBlockchain struct {
	client *Client // Изменили тип на *Client
	logger *zap.Logger
}

func NewSolanaBlockchain(client *Client, logger *zap.Logger) *SolanaBlockchain {
	return &SolanaBlockchain{
		client: client,
		logger: logger,
	}
}

func (s *SolanaBlockchain) Name() string {
	return "Solana"
}

func (s *SolanaBlockchain) SendTransaction(ctx context.Context, tx interface{}) (string, error) {
	// Приводим tx к нужному типу и отправляем транзакцию через клиент Solana
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
