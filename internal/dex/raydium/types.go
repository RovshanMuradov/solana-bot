// internal/dex/raydium/types.go
// Package raydium реализует интеграцию с Raydium DEX на Solana
package raydium

import (
	"context"
	"fmt"
	"math/big"

	"github.com/gagliardetto/solana-go"
	"go.uber.org/zap"
)

// TODO: types.go
// - Добавить типы для новых функций
// - Расширить интерфейсы для поддержки новых операций

// Layout константы для правильного чтения данных из аккаунта пула
const (
	// Базовые смещения
	LayoutDiscriminator = 8
	LayoutStatus        = 1
	LayoutNonce         = 1
	LayoutBaseSize      = LayoutDiscriminator + LayoutStatus + LayoutNonce // 10 байт

	// Смещения для резервов и других данных пула
	LayoutBaseVaultOffset    = LayoutBaseSize + 32 + 32 + 32 // После discriminator, status, nonce и трех pubkeys
	LayoutQuoteVaultOffset   = LayoutBaseVaultOffset + 32
	LayoutBaseReserveOffset  = LayoutQuoteVaultOffset + 32 + 8
	LayoutQuoteReserveOffset = LayoutBaseReserveOffset + 8

	// Константы протокола
	DefaultSwapFeePercent = 0.25
	MinimumAmountOut      = 1
)

const (
	PoolDataSize = 217 // Минимальный размер данных пула

	LayoutAmmProgramID = 2
	LayoutAuthority    = 34
	LayoutBaseVault    = 66
	LayoutQuoteVault   = 98
	LayoutBaseMint     = 130
	LayoutQuoteMint    = 162
	LayoutBaseReserve  = 194
	LayoutQuoteReserve = 202
	LayoutFeeRate      = 210
)

// RaydiumPool представляет собой конфигурацию пула ликвидности Raydium
type RaydiumPool struct {
	// Программы
	AmmProgramID   solana.PublicKey
	SerumProgramID solana.PublicKey

	// AMM конфигурация
	ID            solana.PublicKey // ID пула
	Authority     solana.PublicKey
	OpenOrders    solana.PublicKey
	TargetOrders  solana.PublicKey
	BaseVault     solana.PublicKey
	QuoteVault    solana.PublicKey
	WithdrawQueue solana.PublicKey
	LPVault       solana.PublicKey

	// Токены и минты
	BaseMint      solana.PublicKey
	QuoteMint     solana.PublicKey
	LPMint        solana.PublicKey
	BaseDecimals  uint8
	QuoteDecimals uint8
	LPDecimals    uint8

	// Serum Market
	MarketID         solana.PublicKey
	MarketProgramID  solana.PublicKey
	MarketAuthority  solana.PublicKey
	MarketBaseVault  solana.PublicKey
	MarketQuoteVault solana.PublicKey
	MarketBids       solana.PublicKey
	MarketAsks       solana.PublicKey
	MarketEventQueue solana.PublicKey
	MarketVersion    uint8

	// Lookup Table конфигурация
	LookupTableID        solana.PublicKey
	LookupTableAddresses solana.PublicKeySlice // добавляем это поле

	// Версионирование и инструкции
	Version              uint8
	SwapInstructionIndex uint8
	DefaultMinimumOutBps uint16 // базовых пунктов (1 bps = 0.01%)
	DefaultFeeBps        uint16 // комиссия пула в базовых пунктах

	// Статус пула
	Status uint8
	Nonce  uint8

	BaseReserve  uint64
	QuoteReserve uint64
	FeeRate      uint64
}

// Добавить в types.go:
type LiquidityState struct {
	Status      uint8
	Nonce       uint8
	MaxOrder    *big.Int         // Используется для ограничения размера ордера
	DepthBPS    uint64           // Глубина пула в базисных пунктах
	PnLOwner    solana.PublicKey // Владелец PnL
	ModelDataId solana.PublicKey // ID модели данных
	RecentRoot  *big.Int         // Последний корень для верификации
	OrderBook   OrderBookState
}

type OrderBookState struct {
	Bids       solana.PublicKey
	Asks       solana.PublicKey
	EventQueue solana.PublicKey
	BaseVault  solana.PublicKey
	QuoteVault solana.PublicKey
}

// PoolState содержит динамическое состояние пула
type PoolState struct {
	BaseReserve        uint64
	QuoteReserve       uint64
	SwapFeeNumerator   uint64
	SwapFeeDenominator uint64
	Status             uint8
}

// SwapSide определяет направление свапа
type SwapSide uint8

const (
	SwapSideIn SwapSide = iota
	SwapSideOut
)

// SwapParams содержит параметры для создания инструкций свапа
type SwapParams struct {
	UserWallet          solana.PublicKey
	AmountIn            uint64
	MinAmountOut        uint64
	ComputeUnits        uint32
	PriorityFeeLamports uint64
	LookupTableAccount  *solana.PublicKey // Опционально: адрес lookup таблицы
	WritableIndexes     []uint8           // Индексы для writable аккаунтов в lookup table
	ReadonlyIndexes     []uint8           // Индексы для readonly аккаунтов в lookup table
	Pool                *RaydiumPool      // Информация о пуле
	Logger              *zap.Logger       // Добавляем поле logger

	// Аккаунты для свапа
	SourceTokenAccount      solana.PublicKey
	DestinationTokenAccount solana.PublicKey
}

// Client представляет интерфейс для взаимодействия с Raydium DEX
type Client interface {
	// Основные методы пула
	GetPool(ctx context.Context, poolID solana.PublicKey) (*RaydiumPool, error)
	GetPoolState(ctx context.Context, pool *RaydiumPool) (*PoolState, error)

	// Методы для свапов
	CreateSwapInstructions(ctx context.Context, params SwapParams) ([]solana.Instruction, error)
	SimulateSwap(ctx context.Context, instructions []solana.Instruction) error
	GetAmountOut(pool *RaydiumPool, state *PoolState, amountIn uint64) (uint64, error)
}

// ValidationError представляет ошибку валидации
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation error for %s: %s", e.Field, e.Message)
}

// SwapError представляет ошибку при выполнении свапа
type SwapError struct {
	Stage   string
	Message string
	Err     error
}

func (e *SwapError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("swap error at %s: %s: %v", e.Stage, e.Message, e.Err)
	}
	return fmt.Sprintf("swap error at %s: %s", e.Stage, e.Message)
}

func (e *SwapError) Unwrap() error {
	return e.Err
}

// Типы для v5 пулов
type RaydiumPoolV5 struct {
	// Новые поля v5
}

// Типы для маркет-мейкинга
type MarketMakingParams struct {
	// Параметры для MM
}
