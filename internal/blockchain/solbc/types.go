// internal/blockchain/solbc/types.go
package solbc

import (
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/rovshanmuradov/solana-bot/internal/blockchain"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc/rpc"
)

// TokenMetadataCache кэширует метаданные токенов
type TokenMetadataCache struct {
	cache  sync.Map
	logger *zap.Logger
}

// Client представляет основной клиент Solana
type Client struct {
	rpc     *rpc.RPCClient
	logger  *zap.Logger
	metrics *ClientMetrics
}

type ClientMetrics struct {
	AccountInfoRequests uint64
	TransactionRequests uint64
	FailedRequests      uint64
	LastError           error
	LastErrorTime       time.Time
}

// Проверяем, что Client реализует blockchain.Client интерфейс
var _ blockchain.Client = (*Client)(nil)
