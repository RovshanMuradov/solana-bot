// inernal/dex/raydium/types.go - это пакет, который содержит в себе реализацию работы с декстерами Raydium
package raydium

import (
	"time"

	"github.com/gagliardetto/solana-go"
	solanarpc "github.com/gagliardetto/solana-go/rpc"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain"
	"go.uber.org/zap"
)

// Нужно добавить
type TokenAmount struct {
	Raw      uint64
	Decimals uint8
}

// Нужно добавить
type SwapDirection uint8

const (
	SwapDirectionOut SwapDirection = iota
	SwapDirectionIn
)

// Нужно добавить
type PoolVersion uint8

const (
	PoolVersionUnknown PoolVersion = 0
	PoolVersionV4      PoolVersion = 4
	PoolVersionV3      PoolVersion = 3
)

type Pool struct {
	// Основные поля из блокчейна
	ID            solana.PublicKey // Идентификатор пула
	Authority     solana.PublicKey // Публичный ключ, который имеет полномочия управлять пулом
	BaseMint      solana.PublicKey // Публичный ключ базового токена
	QuoteMint     solana.PublicKey // Публичный ключ котируемого токена
	BaseVault     solana.PublicKey // Публичный ключ хранилища базового токена
	QuoteVault    solana.PublicKey // Публичный ключ хранилища котируемого токена
	BaseDecimals  uint8            // Количество десятичных знаков базового токена
	QuoteDecimals uint8            // Количество десятичных знаков котируемого токена
	DefaultFeeBps uint16           // Комиссия по умолчанию в базисных пунктах (bps)
	Version       PoolVersion
	State         PoolState // встроенное состояние

	// Дополнительные поля из API
	MarketID    solana.PublicKey `json:"marketID"`
	LPMint      solana.PublicKey `json:"lpMint"`
	Creator     solana.PublicKey `json:"creator"`
	TokenSymbol string           `json:"tokenSymbol"`
	TokenName   string           `json:"tokenName"`
	OpenTimeMs  int64            `json:"openTimeMs"`
	Timestamp   time.Time        `json:"timestamp"`
	IsFromAPI   bool             `json:"-"` // Флаг для отслеживания источника данных
}

// Clone создает глубокую копию пула
func (p *Pool) Clone() *Pool {
	if p == nil {
		return nil
	}

	return &Pool{
		ID:            p.ID, // PublicKey можно копировать напрямую, это [32]byte
		Authority:     p.Authority,
		BaseMint:      p.BaseMint,
		QuoteMint:     p.QuoteMint,
		BaseVault:     p.BaseVault,
		QuoteVault:    p.QuoteVault,
		BaseDecimals:  p.BaseDecimals,
		QuoteDecimals: p.QuoteDecimals,
		DefaultFeeBps: p.DefaultFeeBps,
		Version:       p.Version,
		State: PoolState{ // Создаем новую копию PoolState
			BaseReserve:  p.State.BaseReserve,
			QuoteReserve: p.State.QuoteReserve,
			Status:       p.State.Status,
		},
		// API поля
		MarketID:    p.MarketID,
		LPMint:      p.LPMint,
		Creator:     p.Creator,
		TokenSymbol: p.TokenSymbol,
		TokenName:   p.TokenName,
		OpenTimeMs:  p.OpenTimeMs,
		Timestamp:   p.Timestamp,
		IsFromAPI:   p.IsFromAPI,
	}
}

type PoolState struct {
	BaseReserve  uint64 // Резерв базового токена в пуле
	QuoteReserve uint64 // Резерв котируемого токена в пуле
	Status       uint8  // Статус пула (например, активен или неактивен)
}

type SwapParams struct {
	UserWallet              solana.PublicKey   // Публичный ключ кошелька пользователя
	PrivateKey              *solana.PrivateKey // Приватный ключ для подписания транзакции
	AmountIn                uint64             // Количество входного токена
	MinAmountOut            uint64             // Минимальное количество выходного токена
	Pool                    *Pool              // Пул для свапа
	SourceTokenAccount      solana.PublicKey   // Токен аккаунт источника
	DestinationTokenAccount solana.PublicKey   // Токен аккаунт назначения
	PriorityFeeLamports     uint64             // Приоритетная комиссия
	Direction               SwapDirection      // Направление свапа
	SlippageBps             uint16             // Допустимое проскальзывание в базисных пунктах
	WaitConfirmation        bool               // Ожидать ли подтверждения транзакции
}

// SwapResult представляет результат выполнения свапа
type SwapResult struct {
	Signature     solana.Signature
	AmountIn      uint64
	AmountOut     uint64
	FeesPaid      uint64
	ExecutionTime time.Duration
	BlockTime     time.Time
	Confirmed     bool
	RetryCount    int
	Error         error
}

// Client представляет клиент для работы с Raydium DEX
type Client struct {
	client      blockchain.Client
	logger      *zap.Logger
	privateKey  solana.PrivateKey
	timeout     time.Duration
	retries     int
	priorityFee uint64
	commitment  solanarpc.CommitmentType
	poolCache   *PoolCache
	api         *APIService
}
type SwapInstruction struct {
	Amount     *uint64
	MinimumOut *uint64
	Direction  *SwapDirection // добавляем как указатель для консистентности

	// Slice для хранения аккаунтов, следуя паттерну из SDK
	solana.AccountMetaSlice `bin:"-" borsh_skip:"true"`
}

// RaydiumError represents a custom error type
type Error struct {
	Code    string
	Message string
	Stage   string
	Details map[string]interface{}
	Err     error
}

type SwapAmounts struct {
	AmountIn     uint64 // Количество входных токенов
	AmountOut    uint64 // Ожидаемое количество выходных токенов
	MinAmountOut uint64 // Минимальное количество выходных токенов с учетом проскальзывания
}

type Sniper struct {
	client *Client
	logger *zap.Logger
	config *SniperConfig // Конфигурация снайпинга
}

// SniperConfig содержит настройки для снайпера
type SniperConfig struct {
	// Параметры сделок
	MaxSlippageBps   uint16        // Максимальное проскальзывание в базисных пунктах
	MinAmountSOL     uint64        // Минимальная сумма в SOL (в lamports)
	MaxAmountSOL     uint64        // Максимальная сумма в SOL (в lamports)
	PriorityFee      uint64        // Приоритетная комиссия
	WaitConfirmation bool          // Ждать ли подтверждения транзакции
	MonitorInterval  time.Duration // Интервал мониторинга
	MaxRetries       int           // Максимальное количество попыток

	// Параметры токенов
	BaseMint  solana.PublicKey // Mint address базового токена
	QuoteMint solana.PublicKey // Mint address котируемого токена
}

// IsValid проверяет валидность версии пула
func (v PoolVersion) IsValid() bool {
	return v == PoolVersionV3 || v == PoolVersionV4
}

type TokenAccounts struct {
	SourceATA      solana.PublicKey
	DestinationATA solana.PublicKey
	SourceBalance  uint64
	Created        bool
}

type APIMetadata struct {
	Symbol   string
	Name     string
	Decimals uint8
}
