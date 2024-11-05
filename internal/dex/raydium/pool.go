// internal/dex/raydium/pool.go - это пакет, который содержит в себе реализацию работы с пулами Raydium
package raydium

import (
	"context"
	"encoding/binary"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain"
	"go.uber.org/zap"
)

// NewPoolManager создает новый менеджер пула
func NewPoolManager(client blockchain.Client, logger *zap.Logger, pool *Pool) *PoolManager {
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
func (pm *PoolManager) GetPoolState(ctx context.Context) (*PoolState, error) {
	pm.logger.Debug("getting pool state",
		zap.String("poolId", pm.pool.ID.String()),
	)

	// Получаем данные аккаунта пула
	account, err := pm.client.GetAccountInfo(ctx, pm.pool.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get pool account: %w", err)
	}

	// Проверяем что аккаунт существует и содержит данные
	if account == nil || account.Value == nil || account.Value.Data.GetBinary() == nil {
		return nil, fmt.Errorf("pool account data is empty")
	}

	// Получаем бинарные данные аккаунта
	data := account.Value.Data.GetBinary()

	// Проверяем достаточную длину данных
	if len(data) < 89 { // минимальная длина для наших полей
		return nil, fmt.Errorf("invalid pool data length: %d", len(data))
	}

	// Парсим данные в структуру состояния
	state := &PoolState{
		BaseReserve:  binary.LittleEndian.Uint64(data[64:72]), // резервы base токена
		QuoteReserve: binary.LittleEndian.Uint64(data[72:80]), // резервы quote токена
		Status:       data[88],                                // статус пула
	}

	return state, nil
}

// CalculateAmounts рассчитывает количество выходных токенов и минимальный выход
func (pm *PoolManager) CalculateAmounts(ctx context.Context) (*SwapAmounts, error) {
	// Получаем текущее состояние пула
	state, err := pm.GetPoolState(ctx)
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
func (pm *PoolManager) ValidatePool(ctx context.Context) error {
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
	state, err := pm.GetPoolState(ctx)
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
