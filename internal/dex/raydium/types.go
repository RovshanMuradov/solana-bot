// internal/dex/raydium/types.go
package raydium

import (
	"github.com/rovshanmuradov/solana-bot/internal/blockchain"
	"go.uber.org/zap"
)

// RaydiumPoolInfo содержит информацию о пуле Raydium
type Pool struct {
	AmmProgramID               string
	AmmID                      string
	AmmAuthority               string
	AmmOpenOrders              string
	AmmTargetOrders            string
	PoolCoinTokenAccount       string
	PoolPcTokenAccount         string
	SerumProgramID             string
	SerumMarket                string
	SerumBids                  string
	SerumAsks                  string
	SerumEventQueue            string
	SerumCoinVaultAccount      string
	SerumPcVaultAccount        string
	SerumVaultSigner           string
	RaydiumSwapInstructionCode uint8
}

// SwapInstructionData представляет данные инструкции свапа
type SwapInstructionData struct {
	Instruction  uint8  // Тип инструкции
	AmountIn     uint64 // Входящая сумма
	MinAmountOut uint64 // Минимальная исходящая сумма
}
type DEX struct {
	client   blockchain.Client // изменяем тип на интерфейс.
	logger   *zap.Logger
	poolInfo *Pool
	slippage float64 // Добавляем поле для slippage
}

func (r *Pool) GetProgramID() string {
	return r.AmmProgramID
}

func (r *Pool) GetPoolID() string {
	return r.AmmID
}

func (r *Pool) GetTokenAccounts() (string, string) {
	return r.PoolCoinTokenAccount, r.PoolPcTokenAccount
}

// Name возвращает имя DEX
func (r *DEX) Name() string {
	return "Raydium"
}

// PoolState содержит состояние пула ликвидности
type PoolState struct {
	TokenAReserve uint64
	TokenBReserve uint64
	SwapFee       float64 // в процентах
}
