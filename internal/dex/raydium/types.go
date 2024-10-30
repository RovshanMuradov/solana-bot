// internal/dex/raydium/types.go

package raydium

import (
	"sync"
	"sync/atomic"

	"github.com/rovshanmuradov/solana-bot/internal/blockchain"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc"
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
type PoolState struct {
	TokenAReserve uint64
	TokenBReserve uint64
	SwapFee       float64 // в процентах
	CurrentPrice  float64 // текущая цена пула
}

// SwapInstructionData представляет данные инструкции свапа
type SwapInstructionData struct {
	Instruction  uint8  // Тип инструкции
	AmountIn     uint64 // Входящая сумма
	MinAmountOut uint64 // Минимальная исходящая сумма
}
type DEX struct {
	client         blockchain.Client
	logger         *zap.Logger
	poolInfo       *Pool
	slippage       float64
	tokenCache     *solbc.TokenMetadataCache
	priceValidator PriceValidator
	lastPoolState  atomic.Value // Используем atomic.Value для потокобезопасного доступа
	stateMutex     sync.RWMutex // Мьютекс для дополнительной синхронизации при необходимости
}

// setLastPoolState безопасно обновляет состояние пула
func (r *DEX) setLastPoolState(state *PoolState) {
	r.lastPoolState.Store(state)
}

// getLastPoolState безопасно получает состояние пула
func (r *DEX) getLastPoolState() *PoolState {
	return r.lastPoolState.Load().(*PoolState)
}

// UpdatePoolState обновляет состояние пула с дополнительной синхронизацией
func (r *DEX) UpdatePoolState(state *PoolState) {
	r.stateMutex.Lock()
	defer r.stateMutex.Unlock()

	r.setLastPoolState(state)

	// Логируем обновление состояния
	r.logger.Debug("Pool state updated",
		zap.Float64("current_price", state.CurrentPrice),
		zap.Uint64("token_a_reserve", state.TokenAReserve),
		zap.Uint64("token_b_reserve", state.TokenBReserve))
}

// GetPoolStateSnapshot получает снапшот текущего состояния пула
func (r *DEX) GetPoolStateSnapshot() *PoolState {
	r.stateMutex.RLock()
	defer r.stateMutex.RUnlock()

	state := r.getLastPoolState()
	if state == nil {
		return nil
	}

	// Возвращаем копию состояния
	return &PoolState{
		TokenAReserve: state.TokenAReserve,
		TokenBReserve: state.TokenBReserve,
		SwapFee:       state.SwapFee,
		CurrentPrice:  state.CurrentPrice,
	}
}

// Name возвращает имя DEX
func (r *DEX) Name() string {
	return "Raydium"
}
