// =============================
// File: internal/dex/pumpswap/types.go
// =============================
package pumpswap

import (
	"encoding/binary"
	"fmt"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc"
	"github.com/rovshanmuradov/solana-bot/internal/wallet"

	"go.uber.org/zap"
	"sync"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
)

var (
	GlobalConfigDiscriminator = []byte{149, 8, 156, 202, 160, 252, 176, 217}
	PoolDiscriminator         = []byte{241, 154, 109, 4, 17, 177, 109, 188}
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
}

const (
	// Адрес WSOL токена
	WSOLMint = "So11111111111111111111111111111111111111112"

	// Decimals по умолчанию
	DefaultTokenDecimals = 6
	WSOLDecimals         = 9
)

type PreparedTokenAccounts struct {
	UserBaseATA             solana.PublicKey
	UserQuoteATA            solana.PublicKey
	ProtocolFeeRecipientATA solana.PublicKey
	ProtocolFeeRecipient    solana.PublicKey
	CreateBaseATAIx         solana.Instruction
	CreateQuoteATAIx        solana.Instruction
}

// DEX реализует операции для PumpSwap.
type DEX struct {
	client      *solbc.Client
	wallet      *wallet.Wallet
	logger      *zap.Logger
	config      *Config
	poolManager *PoolManager
	rpc         *rpc.Client

	// Новые поля
	globalConfig *GlobalConfig
	configMutex  sync.RWMutex
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

// SlippageExceededError представляет ошибку превышения проскальзывания
type SlippageExceededError struct {
	SlippagePercent float64
	Amount          uint64
	OriginalError   error
}

func ParseGlobalConfig(data []byte) (*GlobalConfig, error) {
	if len(data) < 8 {
		return nil, fmt.Errorf("data too short for GlobalConfig")
	}

	for i := 0; i < 8; i++ {
		if data[i] != GlobalConfigDiscriminator[i] {
			return nil, fmt.Errorf("invalid discriminator for GlobalConfig")
		}
	}

	pos := 8

	if len(data) < pos+32+8+8+1+(32*8) {
		return nil, fmt.Errorf("data too short for GlobalConfig content")
	}

	config := &GlobalConfig{}

	config.Admin = solana.PublicKeyFromBytes(data[pos : pos+32])
	pos += 32

	config.LPFeeBasisPoints = binary.LittleEndian.Uint64(data[pos : pos+8])
	pos += 8

	config.ProtocolFeeBasisPoints = binary.LittleEndian.Uint64(data[pos : pos+8])
	pos += 8

	config.DisableFlags = data[pos]
	pos++

	for i := 0; i < 8; i++ {
		config.ProtocolFeeRecipients[i] = solana.PublicKeyFromBytes(data[pos : pos+32])
		pos += 32
	}

	return config, nil
}
