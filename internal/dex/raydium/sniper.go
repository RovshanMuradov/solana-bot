// internal/dex/raydium/sniper.go
package raydium

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/gagliardetto/solana-go"
	"go.uber.org/zap"
)

// NewSniper создает новый экземпляр снайпера
func NewSniper(client *Client, config *SniperConfig, logger *zap.Logger) *Sniper {
	return &Sniper{
		client: client,
		config: config,
		logger: logger.Named("raydium-sniper"),
	}
}

// ExecuteSnipe выполняет снайпинг транзакцию
func (s *Sniper) ExecuteSnipe(ctx context.Context) error {
	s.logger.Debug("starting snipe execution")

	// 1. Валидация параметров
	if err := s.validateConfig(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	// 2. Получение и валидация пула
	pool, err := s.findAndValidatePool(ctx)
	if err != nil {
		return fmt.Errorf("failed to find and validate pool: %w", err)
	}

	// 3. Определение направления свапа
	direction, err := s.client.DetermineSwapDirection(pool, s.config.BaseMint, s.config.QuoteMint)
	if err != nil {
		return fmt.Errorf("failed to determine swap direction: %w", err)
	}

	// 4. Подготовка параметров свапа
	swapParams, err := s.prepareSwapParams(ctx, pool, direction)
	if err != nil {
		return fmt.Errorf("failed to prepare swap parameters: %w", err)
	}

	// 5. Выполняем свап с механизмом повторных попыток
	sig, err := s.executeSwapWithRetry(ctx, swapParams)
	if err != nil {
		return fmt.Errorf("swap execution failed: %w", err)
	}

	s.logger.Info("snipe executed successfully",
		zap.String("signature", sig.String()),
		zap.Uint64("amount_in", swapParams.AmountIn))

	return nil
}

// Вспомогательные методы для ExecuteSnipe

func (s *Sniper) findAndValidatePool(ctx context.Context) (*Pool, error) {
	// Пробуем получить через API
	pool, err := s.client.api.GetPoolByPair(ctx, s.config.BaseMint, s.config.QuoteMint)
	if err != nil {
		// Проверяем кэш
		pool = s.client.poolCache.GetBestPool(s.config.BaseMint, s.config.QuoteMint)
		if pool == nil {
			// Ищем лучший пул
			pool, err = s.client.FindBestPool(ctx, s.config.BaseMint, s.config.QuoteMint)
			if err != nil {
				return nil, fmt.Errorf("failed to find best pool: %w", err)
			}
		}
	}

	// Проверяем жизнеспособность пула
	if err := s.client.api.IsPoolViable(ctx, pool); err != nil {
		return nil, fmt.Errorf("pool is not viable for sniping: %w", err)
	}

	// Валидируем пару токенов
	if err := s.client.ValidateTokenPair(pool, s.config.BaseMint, s.config.QuoteMint); err != nil {
		return nil, fmt.Errorf("invalid token pair: %w", err)
	}

	return pool, nil
}

func (s *Sniper) prepareSwapParams(ctx context.Context, pool *Pool, direction SwapDirection) (*SwapParams, error) {
	// Подготавливаем токен аккаунты
	accounts, err := s.client.ensureTokenAccounts(ctx, pool.BaseMint, pool.QuoteMint)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare token accounts: %w", err)
	}

	// Рассчитываем параметры свапа
	amounts := CalculateSwapAmounts(pool, s.config.MinAmountSOL, s.config.MaxSlippageBps)

	params := &SwapParams{
		UserWallet:              s.client.GetPublicKey(),
		AmountIn:                amounts.AmountIn,
		MinAmountOut:            amounts.MinAmountOut,
		Pool:                    pool,
		SourceTokenAccount:      accounts.SourceATA,
		DestinationTokenAccount: accounts.DestinationATA,
		PriorityFeeLamports:     s.config.PriorityFee,
		Direction:               direction,
		SlippageBps:             s.config.MaxSlippageBps,
		WaitConfirmation:        s.config.WaitConfirmation,
	}

	// Валидируем условия свапа
	if err := s.client.ValidateSwapConditions(ctx, params); err != nil {
		return nil, fmt.Errorf("swap conditions validation failed: %w", err)
	}

	return params, nil
}

// MonitorPool отслеживает изменения в пуле
func (s *Sniper) MonitorPool(ctx context.Context) error {
	s.logger.Info("starting pool monitoring")

	// Создаем канал для отмены мониторинга
	done := make(chan struct{})
	defer close(done)

	// Создаем ticker для периодических проверок
	liquidityCheckTicker := time.NewTicker(s.config.MonitorInterval)
	defer liquidityCheckTicker.Stop()

	// Запускаем основной мониторинг состояния пула в отдельной горутине
	monitorErrCh := make(chan error, 1)
	go func() {
		initialPool, err := s.client.GetPool(ctx, s.config.BaseMint, s.config.QuoteMint)
		if err != nil {
			monitorErrCh <- fmt.Errorf("failed to get initial pool: %w", err)
			return
		}

		var lastState = initialPool.State
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(500 * time.Millisecond):
				// Получаем актуальное состояние пула
				currentPool, err := s.client.GetPool(ctx, s.config.BaseMint, s.config.QuoteMint)
				if err != nil {
					s.logger.Warn("failed to get pool state", zap.Error(err))
					continue
				}

				// Проверяем значимые изменения
				if s.hasSignificantChanges(&lastState, &currentPool.State) {
					s.logger.Info("significant pool changes detected",
						zap.Uint64("old_base_reserve", lastState.BaseReserve),
						zap.Uint64("new_base_reserve", currentPool.State.BaseReserve),
						zap.Uint64("old_quote_reserve", lastState.QuoteReserve),
						zap.Uint64("new_quote_reserve", currentPool.State.QuoteReserve))

					// Проверяем необходимость выполнения снайпинга
					if s.shouldExecuteSnipe(currentPool) {
						if err := s.ExecuteSnipe(ctx); err != nil {
							s.logger.Error("snipe execution failed", zap.Error(err))
						}
					}
				}

				lastState = currentPool.State

				// Обновляем состояние пула в кэше
				if err := s.client.poolCache.UpdatePoolState(currentPool); err != nil {
					s.logger.Warn("failed to update pool state in cache",
						zap.Error(err),
						zap.String("pool_id", currentPool.ID.String()))
				}
			}
		}
	}()

	// Основной цикл мониторинга
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-monitorErrCh:
			return fmt.Errorf("monitoring error: %w", err)
		case <-liquidityCheckTicker.C:
			currentPool, err := s.client.GetPool(ctx, s.config.BaseMint, s.config.QuoteMint)
			if err != nil {
				s.logger.Warn("failed to get pool state", zap.Error(err))
				continue
			}

			// Проверяем достаточность ликвидности
			if err := CheckLiquiditySufficiency(currentPool, s.config.MinAmountSOL); err != nil {
				s.logger.Warn("insufficient liquidity detected",
					zap.Error(err),
					zap.String("pool_id", currentPool.ID.String()))
				continue
			}
		}
	}
}

// shouldExecuteSnipe определяет, нужно ли выполнять снайпинг
func (s *Sniper) shouldExecuteSnipe(pool *Pool) bool {
	// Проверяем базовые условия
	if pool.State.Status != PoolStatusActive {
		return false
	}

	// Проверяем изменения ликвидности
	baseToQuoteRatio := float64(pool.State.BaseReserve) / float64(pool.State.QuoteReserve)
	const optimalRatioRange = 0.2 // 20% отклонение от 1:1

	isOptimalRatio := math.Abs(1-baseToQuoteRatio) <= optimalRatioRange
	hasLiquidity := pool.State.BaseReserve > s.config.MinAmountSOL*2 &&
		pool.State.QuoteReserve > s.config.MinAmountSOL*2

	return isOptimalRatio && hasLiquidity
}

// Вспомогательные методы

func (s *Sniper) validateConfig() error {
	if s.config.MaxSlippageBps == 0 || s.config.MaxSlippageBps > 10000 {
		return fmt.Errorf("invalid slippage: must be between 0 and 10000")
	}

	if s.config.MinAmountSOL == 0 || s.config.MaxAmountSOL == 0 {
		return fmt.Errorf("invalid amounts")
	}

	if s.config.BaseMint.IsZero() || s.config.QuoteMint.IsZero() {
		return fmt.Errorf("invalid mint addresses")
	}

	if s.config.MonitorInterval < time.Second {
		return fmt.Errorf("monitor interval too small")
	}

	return nil
}

// executeSwapWithRetry выполняет свап с механизмом повторных попыток
func (s *Sniper) executeSwapWithRetry(ctx context.Context, params *SwapParams) (solana.Signature, error) {
	var lastErr error
	var signature solana.Signature

	for attempt := 0; attempt < s.config.MaxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return signature, fmt.Errorf("context cancelled during retry: %w", ctx.Err())
		default:
			// Логируем попытку
			if attempt > 0 {
				s.logger.Info("retrying swap",
					zap.Int("attempt", attempt+1),
					zap.Error(lastErr))
			}

			// Получаем последний блокхэш для новой попытки
			recentBlockHash, err := s.client.GetBaseClient().GetRecentBlockhash(ctx)
			if err != nil {
				lastErr = fmt.Errorf("failed to get recent blockhash: %w", err)
				continue
			}

			// Подготавливаем инструкции
			instructions, err := s.client.PrepareSwapInstructions(params)
			if err != nil {
				lastErr = fmt.Errorf("failed to prepare swap instructions: %w", err)
				continue
			}

			// Создаем транзакцию
			tx, err := solana.NewTransaction(
				instructions,
				recentBlockHash,
				solana.TransactionPayer(params.UserWallet),
			)
			if err != nil {
				lastErr = fmt.Errorf("failed to create transaction: %w", err)
				continue
			}

			// Выполняем свап
			signature, err = s.client.GetBaseClient().SendTransaction(ctx, tx)
			if err == nil {
				// Ждем подтверждения если требуется
				if params.WaitConfirmation {
					if err := s.client.WaitForConfirmation(ctx, signature); err != nil {
						lastErr = fmt.Errorf("confirmation failed: %w", err)
						continue
					}
				}
				return signature, nil
			}
			lastErr = err

			// Экспоненциальная задержка между попытками
			if attempt < s.config.MaxRetries-1 {
				delay := time.Duration(math.Pow(2, float64(attempt))) * time.Second
				select {
				case <-ctx.Done():
					return signature, ctx.Err()
				case <-time.After(delay):
					continue
				}
			}
		}
	}

	return signature, fmt.Errorf("max retries exceeded: %w", lastErr)
}

// hasSignificantChanges определяет, есть ли значимые изменения в состоянии пула
func (s *Sniper) hasSignificantChanges(old, new *PoolState) bool {
	if old == nil || new == nil {
		return false
	}

	// Проверяем изменение статуса
	if old.Status != new.Status {
		return true
	}

	// Рассчитываем изменения в процентах
	baseChange := math.Abs(float64(new.BaseReserve-old.BaseReserve)) / float64(old.BaseReserve)
	quoteChange := math.Abs(float64(new.QuoteReserve-old.QuoteReserve)) / float64(old.QuoteReserve)

	// Пороговые значения для определения значимых изменений
	const (
		significantChangeThreshold = 0.01 // 1% изменение
		criticalChangeThreshold    = 0.05 // 5% изменение
	)

	// Логируем значительные изменения
	if baseChange > significantChangeThreshold || quoteChange > significantChangeThreshold {
		s.logger.Debug("pool changes detected",
			zap.Float64("base_change_percent", baseChange*100),
			zap.Float64("quote_change_percent", quoteChange*100))
	}

	// Возвращаем true если изменения превышают критический порог
	return baseChange > criticalChangeThreshold || quoteChange > criticalChangeThreshold
}
