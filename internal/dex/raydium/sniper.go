// internal/dex/raydium/sniper.go - это пакет, который содержит в себе реализацию снайпинга на декстере Raydium
package raydium

import (
	"fmt"
	"math"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"go.uber.org/zap"
)

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

// TODO: можно добавить:

// 1. Проверку цены перед свапом
// 2. Мониторинг состояния пула в реальном времени
// 3. Более сложную логику расчета суммы свапа
// 4. Обработку различных ошибок и ретраи
// 5. Асинхронное выполнение свапа

func (s *Sniper) ExecuteSnipe() error {
	s.logger.Debug("starting snipe execution")

	// 1. Получение пула и валидация параметров
	if err := s.ValidateAndPrepare(); err != nil {
		return fmt.Errorf("failed to validate parameters: %w", err)
	}

	// 2. Получение информации о пуле и проверка его состояния
	pool, err := s.client.GetPool(s.config.baseMint, s.config.quoteMint)
	if err != nil {
		return fmt.Errorf("failed to get pool: %w", err)
	}

	poolManager := NewPoolManager(s.client.client, s.logger, pool)
	if err := poolManager.ValidatePool(); err != nil {
		return fmt.Errorf("pool validation failed: %w", err)
	}

	// 3. Расчет параметров свапа
	amounts, err := poolManager.CalculateAmounts()
	if err != nil {
		return fmt.Errorf("failed to calculate swap amounts: %w", err)
	}

	// 4. Подготовка параметров для свапа
	swapParams := &SwapParams{
		UserWallet:          s.client.client.PrivateKey.PublicKey(),
		AmountIn:            amounts.AmountIn,
		MinAmountOut:        amounts.MinAmountOut,
		Pool:                pool,
		PriorityFeeLamports: s.config.priorityFee,
		// Здесь нужно добавить source и destination token accounts,
		// которые должны быть получены или созданы заранее
	}

	// 5. Выполнение свапа
	signature, err := s.client.ExecuteSwap(swapParams)
	if err != nil {
		return fmt.Errorf("swap execution failed: %w", err)
	}

	// Логируем успешное выполнение
	s.logger.Info("snipe executed successfully",
		zap.String("signature", signature),
		zap.Uint64("amountIn", amounts.AmountIn),
		zap.Uint64("amountOut", amounts.AmountOut),
		zap.Uint64("minAmountOut", amounts.MinAmountOut),
	)

	return nil
}

// TODO: Потенциальные улучшения на основе TS версии:
// 1. Добавить проверку и создание associated token accounts
// 2. Добавить проверку балансов SOL и токенов
// 3. Добавить валидацию параметров compute budget
// 4. Добавить проверку версии пула (V4)
// 5. Добавить расчет приоритетной комиссии на основе последних блоков
// 6. Добавить проверку и обработку wrapped SOL
func (s *Sniper) ValidateAndPrepare() error {
	s.logger.Debug("validating and preparing snipe parameters")

	// Проверяем базовые параметры конфигурации
	if s.config.maxSlippageBps == 0 || s.config.maxSlippageBps > 10000 { // 10000 = 100%
		return fmt.Errorf("invalid slippage: must be between 0 and 10000")
	}

	if s.config.minAmountSOL <= 0 || s.config.maxAmountSOL <= 0 {
		return fmt.Errorf("invalid amount parameters")
	}

	if s.config.maxAmountSOL < s.config.minAmountSOL {
		return fmt.Errorf("maxAmount cannot be less than minAmount")
	}

	// Проверяем mint addresses
	if s.config.baseMint.IsZero() || s.config.quoteMint.IsZero() {
		return fmt.Errorf("invalid mint addresses")
	}

	// Проверяем наличие достаточного баланса
	balance, err := s.client.client.GetBalance(
		s.client.client.PrivateKey.PublicKey(),
		rpc.CommitmentConfirmed,
	)
	if err != nil {
		return fmt.Errorf("failed to get wallet balance: %w", err)
	}

	if float64(balance.Value)/float64(solana.LAMPORTS_PER_SOL) < s.config.minAmountSOL {
		return fmt.Errorf("insufficient balance")
	}

	// Проверяем параметры мониторинга
	if s.config.monitorInterval < time.Second {
		return fmt.Errorf("monitor interval too small")
	}

	if s.config.maxRetries < 1 {
		return fmt.Errorf("invalid max retries value")
	}

	return nil
}

// TODO: Потенциальные улучшения на основе TS версии:
// 1. Добавить отслеживание изменений цены
// 2. Добавить отслеживание объема ликвидности
// 3. Добавить механизм подписки на события пула
// 4. Добавить отслеживание транзакций в мемпуле
// 5. Добавить механизм websocket подключения
// 6. Добавить механизм агрегации данных по нескольким RPC
func (s *Sniper) MonitorPoolChanges() error {
	s.logger.Debug("starting pool monitoring")

	ticker := time.NewTicker(s.config.monitorInterval)
	defer ticker.Stop()

	// Получаем начальное состояние пула
	pool, err := s.client.GetPool(s.config.baseMint, s.config.quoteMint)
	if err != nil {
		return fmt.Errorf("failed to get initial pool state: %w", err)
	}

	poolManager := NewPoolManager(s.client.client, s.logger, pool)
	initialState, err := poolManager.GetPoolState()
	if err != nil {
		return fmt.Errorf("failed to get initial pool state: %w", err)
	}

	var retryCount int
	for {
		select {
		case <-ticker.C:
			// Получаем текущее состояние пула
			currentState, err := poolManager.GetPoolState()
			if err != nil {
				retryCount++
				s.logger.Error("failed to get current pool state",
					zap.Error(err),
					zap.Int("retry", retryCount),
				)
				if retryCount >= s.config.maxRetries {
					return fmt.Errorf("max retries exceeded while monitoring pool")
				}
				continue
			}
			retryCount = 0

			// Проверяем изменения в пуле
			if s.hasSignificantChanges(initialState, currentState) {
				s.logger.Info("detected significant pool changes",
					zap.Uint64("oldBaseReserve", initialState.BaseReserve),
					zap.Uint64("newBaseReserve", currentState.BaseReserve),
					zap.Uint64("oldQuoteReserve", initialState.QuoteReserve),
					zap.Uint64("newQuoteReserve", currentState.QuoteReserve),
				)

				// Если пул неактивен, прекращаем мониторинг
				if currentState.Status != 1 {
					return fmt.Errorf("pool became inactive")
				}

				// Обновляем начальное состояние
				initialState = currentState
			}
		}
	}
}

// Вспомогательный метод для определения значительных изменений в пуле
func (s *Sniper) hasSignificantChanges(old, new *PoolState) bool {
	// Рассчитываем процент изменения для базового резерва
	baseChange := math.Abs(float64(new.BaseReserve)-float64(old.BaseReserve)) / float64(old.BaseReserve)

	// Рассчитываем процент изменения для котируемого резерва
	quoteChange := math.Abs(float64(new.QuoteReserve)-float64(old.QuoteReserve)) / float64(old.QuoteReserve)

	// Определяем порог значительных изменений (например, 1%)
	threshold := 0.01

	return baseChange > threshold || quoteChange > threshold || new.Status != old.Status
}
