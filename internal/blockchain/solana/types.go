// internal/blockchain/solana/types.go
package solana

import (
	"context"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"go.uber.org/zap"
)

// SolanaClientInterface определяет интерфейс для клиента
type SolanaClientInterface interface {
	GetRecentBlockhash(ctx context.Context) (solana.Hash, error)
	SendTransaction(ctx context.Context, tx *solana.Transaction) (solana.Signature, error)
}

// Client реализует SolanaClientInterface
type Client struct {
	rpcPool *RPCPool
	logger  *zap.Logger
}

// ISolanaClient определяет интерфейс для взаимодействия с Solana
type ISolanaClient interface {
	GetRecentBlockhash(ctx context.Context) (solana.Hash, error)
	SendTransaction(ctx context.Context, tx *solana.Transaction) (solana.Signature, error)
}

// IRPCClient определяет интерфейс для RPC клиента
type IRPCClient interface {
	GetRecentBlockhash(ctx context.Context, commitment rpc.CommitmentType) (rpc.GetRecentBlockhashResult, error)
	SendTransactionWithOpts(ctx context.Context, tx *solana.Transaction, opts rpc.TransactionOpts) (solana.Signature, error)
}

// IBlockchain определяет интерфейс для blockchain операций
type IBlockchain interface {
	Name() string
	SendTransaction(ctx context.Context, tx interface{}) (string, error)
}
