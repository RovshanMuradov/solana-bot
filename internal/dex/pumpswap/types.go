// =============================
// File: internal/dex/pumpswap/types.go
// =============================
package pumpswap

import (
	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc"
	"github.com/rovshanmuradov/solana-bot/internal/wallet"
	"time"

	"go.uber.org/zap"
	"sync"

	"github.com/gagliardetto/solana-go"
)

const (
	// Decimals по умолчанию
	DefaultTokenDecimals = 6
	WSOLDecimals         = 9
)

var (
	GlobalConfigDiscriminator = []byte{149, 8, 156, 202, 160, 252, 176, 217}
	PoolDiscriminator         = []byte{241, 154, 109, 4, 17, 177, 109, 188}
)
var (
	// PumpSwapProgramID – адрес программы PumpSwap.
	PumpSwapProgramID = solana.MustPublicKeyFromBase58("pAMMBay6oceH9fJKBRHGP5D4bD4sWpmSwMn52FMfXEA")
	// SystemProgramID – ID системной программы Solana.
	SystemProgramID = solana.SystemProgramID
	// TokenProgramID – ID программы токенов Solana.
	TokenProgramID = solana.TokenProgramID
	// AssociatedTokenProgramID – ID ассоциированной токенной программы.
	AssociatedTokenProgramID = solana.SPLAssociatedTokenAccountProgramID
)

type GlobalConfig struct {
	Admin                  solana.PublicKey
	LPFeeBasisPoints       uint64
	ProtocolFeeBasisPoints uint64
	DisableFlags           uint8
	ProtocolFeeRecipients  [8]solana.PublicKey
}

type Pool struct {
	PoolBump              uint8
	Index                 uint16
	Creator               solana.PublicKey
	BaseMint              solana.PublicKey
	QuoteMint             solana.PublicKey
	LPMint                solana.PublicKey
	PoolBaseTokenAccount  solana.PublicKey
	PoolQuoteTokenAccount solana.PublicKey
	LPSupply              uint64
	CoinCreator           solana.PublicKey
}

type PoolInfo struct {
	Address               solana.PublicKey
	BaseMint              solana.PublicKey
	QuoteMint             solana.PublicKey
	BaseReserves          uint64
	QuoteReserves         uint64
	LPSupply              uint64
	FeesBasisPoints       uint64
	ProtocolFeeBPS        uint64
	LPMint                solana.PublicKey
	PoolBaseTokenAccount  solana.PublicKey
	PoolQuoteTokenAccount solana.PublicKey
	CoinCreator           solana.PublicKey
}

type PreparedTokenAccounts struct {
	UserBaseATA               solana.PublicKey
	UserQuoteATA              solana.PublicKey
	ProtocolFeeRecipientATA   solana.PublicKey
	ProtocolFeeRecipient      solana.PublicKey
	CoinCreatorVaultATA       solana.PublicKey
	CoinCreatorVaultAuthority solana.PublicKey
	CreateBaseATAIx           solana.Instruction
	CreateQuoteATAIx          solana.Instruction
}

// DEX реализует операции для PumpSwap.
type DEX struct {
	client       *solbc.Client
	wallet       *wallet.Wallet
	logger       *zap.Logger
	config       *Config
	poolManager  PoolManagerInterface
	globalConfig *GlobalConfig
	configMutex  sync.RWMutex

	// Кэшированные данные для оптимизации запросов
	cachedPool       *PoolInfo
	cachedPoolTime   time.Time
	cachedPrice      float64
	cachedPriceTime  time.Time
	cacheValidPeriod time.Duration
}

// SwapAmounts содержит результаты расчёта параметров свапа
type SwapAmounts struct {
	BaseAmount  uint64  // Сумма базовой валюты
	QuoteAmount uint64  // Сумма котируемой валюты
	Price       float64 // Расчётная цена
}

// Определяем SwapParams локально в пакете pumpswap
type SwapParams struct {
	IsBuy           bool
	Amount          uint64
	SlippagePercent float64
	PriorityFeeSol  string
	ComputeUnits    uint32
}
