// internal/dex/raydium/pool.go - это пакет, который содержит в себе реализацию работы с пулами Raydium
package raydium

import (
	"encoding/binary"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain"
	"go.uber.org/zap"
)

// NewPoolManager создает новый менеджер пула
func NewPoolManager(client blockchain.Client, logger *zap.Logger, pool *RaydiumPool) *PoolManager {
	return &PoolManager{
		client: client,
		logger: logger,
		pool:   pool,
	}
}

// TODO: В дальнейшем этот код можно расширить:
// 1. Добавить более сложную формулу расчета с учетом комиссий
// 2. Реализовать кэширование состояния пула
// 3. Добавить больше проверок валидации
// 4. Улучшить обработку ошибок и логирование

// GetPoolState получает актуальное состояние пула
func (pm *PoolManager) GetPoolState() (*PoolState, error) {
	pm.logger.Debug("getting pool state",
		zap.String("poolId", pm.pool.ID.String()),
	)

	// Получаем данные аккаунта пула
	account, err := pm.client.GetAccountInfo(pm.pool.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get pool account: %w", err)
	}

	// Парсим данные в структуру состояния
	state := &PoolState{
		BaseReserve:  binary.LittleEndian.Uint64(account.Data[64:72]), // резервы base токена
		QuoteReserve: binary.LittleEndian.Uint64(account.Data[72:80]), // резервы quote токена
		Status:       account.Data[88],                                // статус пула
	}

	return state, nil
}

// CalculateAmounts рассчитывает количество выходных токенов и минимальный выход
func (pm *PoolManager) CalculateAmounts() (*SwapAmounts, error) {
	// Получаем текущее состояние пула
	state, err := pm.GetPoolState()
	if err != nil {
		return nil, fmt.Errorf("failed to get pool state: %w", err)
	}

	// Проверяем, что пул активен
	if state.Status != 1 { // предполагаем, что 1 = активный статус
		return nil, fmt.Errorf("pool is not active")
	}

	// Расчет по формуле: amountOut = (amountIn * outputReserve) / (inputReserve + amountIn)
	// Это упрощенная формула для начала
	amountIn := uint64(1000000) // пример входного количества
	amountOut := (amountIn * state.QuoteReserve) / (state.BaseReserve + amountIn)

	// Учитываем проскальзывание (например, 1%)
	slippage := uint64(100) // 1%
	minAmountOut := amountOut - (amountOut * slippage / 10000)

	return &SwapAmounts{
		AmountIn:     amountIn,
		AmountOut:    amountOut,
		MinAmountOut: minAmountOut,
	}, nil
}

// ValidatePool проверяет валидность параметров пула
func (pm *PoolManager) ValidatePool() error {
	// Проверяем существование всех необходимых аккаунтов
	accounts := []solana.PublicKey{
		pm.pool.ID,
		pm.pool.Authority,
		pm.pool.BaseMint,
		pm.pool.QuoteMint,
		pm.pool.BaseVault,
		pm.pool.QuoteVault,
	}

	for _, acc := range accounts {
		if acc.IsZero() {
			return fmt.Errorf("invalid pool account: %s is zero", acc.String())
		}
	}

	// Проверяем состояние пула
	state, err := pm.GetPoolState()
	if err != nil {
		return fmt.Errorf("failed to get pool state: %w", err)
	}

	// Проверяем резервы
	if state.BaseReserve == 0 || state.QuoteReserve == 0 {
		return fmt.Errorf("pool reserves are empty: base=%d, quote=%d",
			state.BaseReserve, state.QuoteReserve)
	}

	// Проверяем статус
	if state.Status != 1 { // предполагаем, что 1 = активный статус
		return fmt.Errorf("pool is not active")
	}

	return nil
}
