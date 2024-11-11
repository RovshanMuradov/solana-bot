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
type SwapDirection string

const (
	SwapDirectionIn  SwapDirection = "in"
	SwapDirectionOut SwapDirection = "out"
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

type PoolState struct {
	BaseReserve  uint64 // Резерв базового токена в пуле
	QuoteReserve uint64 // Резерв котируемого токена в пуле
	Status       uint8  // Статус пула (например, активен или неактивен)
}

type SwapParams struct {
	UserWallet              solana.PublicKey   // Публичный ключ кошелька пользователя
	PrivateKey              *solana.PrivateKey // Приватный ключ для подписания транзакции
	AmountIn                uint64             // Количество входного токена для обмена
	MinAmountOut            uint64             // Минимальное количество выходного токена
	Pool                    *Pool              // Указатель на пул для обмена
	SourceTokenAccount      solana.PublicKey   // Аккаунт исходного токена
	DestinationTokenAccount solana.PublicKey   // Аккаунт целевого токена
	PriorityFeeLamports     uint64             // Приоритетная комиссия в лампортах

	Direction   SwapDirection
	SlippageBps uint16
	Deadline    time.Time // таймаут для транзакции
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

// RaydiumSwapInstruction реализует интерфейс solana.Instruction
type ExecutableSwapInstruction struct {
	programID solana.PublicKey
	accounts  []*solana.AccountMeta
	data      []byte
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
type SniperConfig struct {
	// Существующие поля
	MaxSlippageBps   uint16 // экспортируемые поля
	MinAmountSOL     uint64 // использовать lamports вместо float64
	MaxAmountSOL     uint64
	PriorityFee      uint64
	WaitConfirmation bool
	MonitorInterval  time.Duration
	MaxRetries       int

	// Добавляем новые необходимые поля
	BaseMint  solana.PublicKey // Mint address базового токена
	QuoteMint solana.PublicKey // Mint address котируемого токена
}

// IsValid проверяет валидность версии пула
func (v PoolVersion) IsValid() bool {
	return v == PoolVersionV3 || v == PoolVersionV4
}
