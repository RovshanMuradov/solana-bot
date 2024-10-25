package solana

import (
	"context"
	"sync"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"go.uber.org/zap"
)

const (
	defaultTimeout = 10 * time.Second
	maxRetries     = 3
	retryDelay     = 1 * time.Second
)

// SolanaClientInterface определяет интерфейс для клиента
type SolanaClientInterface interface {
	GetRecentBlockhash(ctx context.Context) (solana.Hash, error)
	SendTransaction(ctx context.Context, tx *solana.Transaction) (solana.Signature, error)
}

// RPCClient представляет отдельный RPC узел
type RPCClient struct {
	Client  *rpc.Client
	URL     string
	active  bool
	mutex   sync.RWMutex
	metrics *RPCMetrics
}

// RPCMetrics содержит метрики производительности RPC узла
type RPCMetrics struct {
	successCount uint64
	errorCount   uint64
	latency      time.Duration
	mutex        sync.RWMutex
}

// Client представляет основной клиент Solana
type Client struct {
	rpcClients []*RPCClient
	logger     *zap.Logger
	currIndex  int
	mutex      sync.Mutex
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
