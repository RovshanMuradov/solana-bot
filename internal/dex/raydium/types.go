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
// Обновляем также структуру инструкции
type SwapInstructionData struct {
	Instruction  uint8 // Изменено на uint8
	AmountIn     uint64
	MinAmountOut uint64
}

// internal/dex/raydium/types.go
type DEX struct {
	client   blockchain.Client // изменяем тип на интерфейс.
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
