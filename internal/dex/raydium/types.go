// internal/dex/raydium/types.go
package raydium

import (
	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solana"
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
	RaydiumSwapInstructionCode uint64
}

// SwapInstructionData представляет данные инструкции свапа
type SwapInstructionData struct {
	Instruction  uint64 // Код инструкции
	AmountIn     uint64 // Сумма входа
	MinAmountOut uint64 // Минимальная сумма выхода
}

// internal/dex/raydium/types.go
type DEX struct {
	client   solana.SolanaClientInterface // изменяем тип на интерфейс
	logger   *zap.Logger
	poolInfo *Pool
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
