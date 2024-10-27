// internal/blockchain/solbc/rpc/types.go
package rpc

import (
	"sync"
	"time"

	"github.com/gagliardetto/solana-go/rpc"
	"go.uber.org/zap"
)

const (
	DefaultTimeout = 10 * time.Second
	MaxRetries     = 3
	RetryDelay     = 1 * time.Second
)

// NodeClient представляет отдельный RPC узел
type NodeClient struct {
	Client  *rpc.Client
	URL     string
	active  bool
	mutex   sync.RWMutex
	metrics *metrics
}

// metrics содержит метрики производительности RPC узла
type metrics struct {
	successCount uint64
	errorCount   uint64
	latency      time.Duration
	mutex        sync.RWMutex
}

// Pool представляет пул RPC клиентов
type Pool struct {
	Clients   []*NodeClient
	Logger    *zap.Logger
	CurrIndex int
	Mutex     sync.Mutex
}
