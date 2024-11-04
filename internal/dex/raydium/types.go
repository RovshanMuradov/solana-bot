// inernal/dex/raydium/types.go - это пакет, который содержит в себе реализацию работы с декстерами Raydium
package raydium

import (
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain"
	"go.uber.org/zap"
)

type RaydiumPool struct {
	ID            solana.PublicKey // Идентификатор пула
	Authority     solana.PublicKey // Публичный ключ, который имеет полномочия управлять пулом
	BaseMint      solana.PublicKey // Публичный ключ базового токена
	QuoteMint     solana.PublicKey // Публичный ключ котируемого токена
	BaseVault     solana.PublicKey // Публичный ключ хранилища базового токена
	QuoteVault    solana.PublicKey // Публичный ключ хранилища котируемого токена
	BaseDecimals  uint8            // Количество десятичных знаков базового токена
	QuoteDecimals uint8            // Количество десятичных знаков котируемого токена
	DefaultFeeBps uint16           // Комиссия по умолчанию в базисных пунктах (bps)
	// Только необходимые поля для V4
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
	Pool                    *RaydiumPool       // Указатель на пул для обмена
	SourceTokenAccount      solana.PublicKey   // Аккаунт исходного токена
	DestinationTokenAccount solana.PublicKey   // Аккаунт целевого токена
	PriorityFeeLamports     uint64             // Приоритетная комиссия в лампортах
}

// Основные ошибки
type SwapError struct {
	Stage   string // Этап, на котором произошла ошибка
	Message string // Сообщение об ошибке
	Err     error  // Вложенная ошибка
}

type RaydiumClient struct {
	client     blockchain.Client
	logger     *zap.Logger
	options    *clientOptions // Базовые настройки таймаутов и retry
	privateKey solana.PrivateKey
}
type clientOptions struct {
	timeout     time.Duration      // Таймаут для операций
	retries     int                // Количество повторных попыток
	priorityFee uint64             // Приоритетная комиссия в лампортах
	commitment  rpc.CommitmentType // Уровень подтверждения транзакций
}

// Вспомогательные структуры для инструкций
type ComputeBudgetInstruction struct {
	Units         uint32
	MicroLamports uint64
}

type SwapInstruction struct {
	Amount     *uint64
	MinimumOut *uint64

	// Slice для хранения аккаунтов, следуя паттерну из SDK
	solana.AccountMetaSlice `bin:"-" borsh_skip:"true"`
}

// RaydiumSwapInstruction реализует интерфейс solana.Instruction
type RaydiumSwapInstruction struct {
	programID solana.PublicKey
	accounts  []*solana.AccountMeta
	data      []byte
}

// RaydiumError represents a custom error type
type RaydiumError struct {
	Code    string
	Message string
	Details map[string]interface{}
}

type SwapAmounts struct {
	AmountIn     uint64 // Количество входных токенов
	AmountOut    uint64 // Ожидаемое количество выходных токенов
	MinAmountOut uint64 // Минимальное количество выходных токенов с учетом проскальзывания
}

type PoolManager struct {
	client blockchain.Client
	logger *zap.Logger
	pool   *RaydiumPool
}

type Sniper struct {
	client *RaydiumClient
	logger *zap.Logger
	config *SniperConfig // Конфигурация снайпинга
}
type SniperConfig struct {
	// Существующие поля
	maxSlippageBps   uint16
	minAmountSOL     float64
	maxAmountSOL     float64
	priorityFee      uint64
	waitConfirmation bool
	monitorInterval  time.Duration
	maxRetries       int

	// Добавляем новые необходимые поля
	baseMint  solana.PublicKey // Mint address базового токена
	quoteMint solana.PublicKey // Mint address котируемого токена
}
