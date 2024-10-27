// internal/blockchain/solbc/types.go
package solbc

import (
	"sync"

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
	rpcPool *rpc.Pool
	logger  *zap.Logger
}

// Проверяем, что Client реализует blockchain.Client интерфейс
var _ blockchain.Client = (*Client)(nil)
