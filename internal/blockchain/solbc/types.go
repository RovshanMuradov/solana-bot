// internal/blockchain/solana/types.go
package solbc

import (
	"sync"
	"time"

	"github.com/gagliardetto/solana-go/rpc"
	"go.uber.org/zap"

	"github.com/rovshanmuradov/solana-bot/internal/blockchain"
)

const (
	defaultTimeout = 10 * time.Second
	maxRetries     = 3
	retryDelay     = 1 * time.Second
)

// RPCNodeClient представляет отдельный RPC узел
type RPCNodeClient struct {
	Client  *rpc.Client
	URL     string
	active  bool
	mutex   sync.RWMutex
	metrics *RPCMetrics
}

// TokenMetadataCache кэширует метаданные токенов
type TokenMetadataCache struct {
	cache  sync.Map
	logger *zap.Logger
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
	rpcClients []*RPCNodeClient // Оставляем оригинальное имя
	logger     *zap.Logger
	currIndex  int
	mutex      sync.Mutex
}

// Проверяем, что Client реализует blockchain.Client интерфейс
var _ blockchain.Client = (*Client)(nil)
