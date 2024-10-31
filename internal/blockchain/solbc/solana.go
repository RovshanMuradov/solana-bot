// internal/blockchain/solbc/solana.go
package solbc

import (
	"context"
	"fmt"

	solanarpc "github.com/gagliardetto/solana-go/rpc"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc/rpc"

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

func (c *Client) GetRpcClient() *solanarpc.Client {
	if c.adapter == nil {
		c.adapter = NewRpcAdapter(c.rpc)
	}
	return c.adapter
}

// RpcAdapter адаптирует наш RPCClient к интерфейсу solana-go/rpc.Client
type RpcAdapter struct {
	client *rpc.RPCClient
}

func NewRpcAdapter(client *rpc.RPCClient) *solanarpc.Client {
	return solanarpc.New("") // Создаем пустой клиент
}
