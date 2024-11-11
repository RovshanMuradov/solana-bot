// internal/blockchain/solbc/types.go
package solbc

import (
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/gagliardetto/solana-go"
	solanarpc "github.com/gagliardetto/solana-go/rpc"
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
	rpc        *rpc.Client
	adapter    *solanarpc.Client
	logger     *zap.Logger
	metrics    *ClientMetrics
	privateKey solana.PrivateKey // Добавляем поле для приватного ключа
}

type ClientMetrics struct {
	AccountInfoRequests     uint64
	TransactionRequests     uint64
	FailedRequests          uint64
	ProgramAccountRequests  uint64
	LastError               error
	LastErrorTime           time.Time
	BalanceRequests         uint64
	SuccessfulConfirmations int64
}

// IncrementProgramAccountRequests атомарно увеличивает счетчик запросов
func (m *ClientMetrics) IncrementProgramAccountRequests() {
	atomic.AddUint64(&m.ProgramAccountRequests, 1)
}

// IncrementFailedRequests атомарно увеличивает счетчик ошибок
func (m *ClientMetrics) IncrementFailedRequests() {
	atomic.AddUint64(&m.FailedRequests, 1)
}
func (m *ClientMetrics) IncrementBalanceRequests() {
	atomic.AddUint64(&m.BalanceRequests, 1)
}

// Проверяем, что Client реализует blockchain.Client интерфейс
var _ blockchain.Client = (*Client)(nil)
