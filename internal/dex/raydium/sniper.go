// internal/dex/raydium/sniper.go - это пакет, который содержит в себе реализацию снайпинга на декстере Raydium
package raydium

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/gagliardetto/solana-go"
	solanarpc "github.com/gagliardetto/solana-go/rpc"
	"go.uber.org/zap"
)

// TODO: можно добавить:

// 1. Проверку цены перед свапом
// 2. Мониторинг состояния пула в реальном времени
// 3. Более сложную логику расчета суммы свапа
// 4. Обработку различных ошибок и ретраи
// 5. Асинхронное выполнение свапа

func (s *Sniper) ExecuteSnipe() error {
	ctx := context.Background()
	s.logger.Debug("starting snipe execution")

	// 1. Валидация параметров
	if err := s.ValidateAndPrepare(); err != nil {
		return fmt.Errorf("failed to validate parameters: %w", err)
	}

	// 2. Получение информации о пуле через API и кэш
	pool, err := s.client.GetPool(ctx, s.config.BaseMint, s.config.QuoteMint)
	if err != nil {
		// Пробуем в обратном порядке
		pool, err = s.client.GetPool(ctx, s.config.QuoteMint, s.config.BaseMint)
		if err != nil {
			return fmt.Errorf("failed to get pool in both directions: %w", err)
		}
	}

	// 3. Проверяем состояние пула
	if !IsPoolActive(pool) {
		return fmt.Errorf("pool is not active or has no liquidity")
	}

	// 4. Получаем Associated Token Accounts
	sourceATA, _, err := solana.FindAssociatedTokenAddress(
		s.client.privateKey.PublicKey(),
		s.config.BaseMint,
	)
	if err != nil {
		return fmt.Errorf("failed to get source ATA: %w", err)
	}

	destinationATA, _, err := solana.FindAssociatedTokenAddress(
		s.client.privateKey.PublicKey(),
		s.config.QuoteMint,
	)
	if err != nil {
		return fmt.Errorf("failed to get destination ATA: %w", err)
	}

	// 5. Расчет параметров свапа
	amounts := CalculateSwapAmounts(pool, s.config.MinAmountSOL, s.config.MaxSlippageBps)

	// Проверяем влияние на цену
	priceImpact := GetPriceImpact(pool, amounts.AmountIn)
	if priceImpact > 5.0 { // 5% максимальное влияние на цену
		return fmt.Errorf("price impact too high: %.2f%%", priceImpact)
	}

	// 6. Проверяем минимальные и максимальные лимиты
	if amounts.AmountIn < s.config.MinAmountSOL {
		return fmt.Errorf("amount too small: %d < %d", amounts.AmountIn, s.config.MinAmountSOL)
	}
	if amounts.AmountIn > s.config.MaxAmountSOL {
		return fmt.Errorf("amount too large: %d > %d", amounts.AmountIn, s.config.MaxAmountSOL)
	}

	// 7. Проверяем баланс
	balance, err := s.client.CheckWalletBalance(ctx)
	if err != nil {
		return fmt.Errorf("failed to check balance: %w", err)
	}

	requiredBalance := amounts.AmountIn + s.config.PriorityFee + 5000
	if balance < requiredBalance {
		return fmt.Errorf("insufficient balance: required %d, got %d", requiredBalance, balance)
	}

	// 8. Подготовка параметров для свапа
	swapParams := &SwapParams{
		UserWallet:              s.client.privateKey.PublicKey(),
		PrivateKey:              &s.client.privateKey,
		AmountIn:                amounts.AmountIn,
		MinAmountOut:            amounts.MinAmountOut,
		Pool:                    pool,
		SourceTokenAccount:      sourceATA,
		DestinationTokenAccount: destinationATA,
		PriorityFeeLamports:     s.config.PriorityFee,
		Direction:               SwapDirectionIn,
		SlippageBps:             s.config.MaxSlippageBps,
		Deadline:                time.Now().Add(30 * time.Second),
	}

	// 9. Симуляция свапа
	if err := s.client.SimulateSwap(ctx, swapParams); err != nil {
		return fmt.Errorf("swap simulation failed: %w", err)
	}

	// 10. Выполнение свапа
	signature, err := s.client.ExecuteSwap(swapParams)
	if err != nil {
		return fmt.Errorf("swap execution failed: %w", err)
	}

	// 11. Ждем подтверждения если настроено
	if s.config.WaitConfirmation {
		s.logger.Info("waiting for confirmation...",
			zap.String("signature", signature))

		status, err := s.waitForConfirmation(ctx, signature)
		if err != nil {
			return fmt.Errorf("confirmation failed: %w", err)
		}
		if status != "confirmed" && status != "finalized" {
			return fmt.Errorf("transaction failed with status: %s", status)
		}
	}

	s.logger.Info("snipe executed successfully",
		zap.String("signature", signature),
		zap.Uint64("amountIn", amounts.AmountIn),
		zap.Uint64("amountOut", amounts.AmountOut),
		zap.Uint64("minAmountOut", amounts.MinAmountOut),
		zap.Float64("price_impact", priceImpact),
		zap.String("explorer", fmt.Sprintf("https://explorer.solana.com/tx/%s", signature)),
	)

	return nil
}

// TODO: Потенциальные улучшения на основе TS версии:
// 1. Добавить отслеживание изменений цены
// 2. Добавить отслеживание объема ликвидности
// 3. Добавить механизм подписки на события пула
// 4. Добавить отслеживание транзакций в мемпуле
// 5. Добавить механизм websocket подключения
// 6. Добавить механизм агрегации данных по нескольким RPC
func (s *Sniper) MonitorPoolChanges(ctx context.Context) error {
	s.logger.Debug("starting pool monitoring")

	ticker := time.NewTicker(s.config.MonitorInterval)
	defer ticker.Stop()

	// Получаем начальное состояние пула
	pool, err := s.client.GetPool(ctx, s.config.BaseMint, s.config.QuoteMint)
	if err != nil {
		return fmt.Errorf("failed to get initial pool state: %w", err)
	}

	if !IsPoolActive(pool) {
		return fmt.Errorf("initial pool is not active or has no liquidity")
	}

	initialState := pool.State
	var retryCount int

	for range ticker.C {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("monitoring stopped: %w", err)
		}

		// Получаем актуальное состояние пула
		currentPool, err := s.client.GetPool(ctx, s.config.BaseMint, s.config.QuoteMint)
		if err != nil {
			retryCount++
			s.logger.Error("failed to get current pool state",
				zap.Error(err),
				zap.Int("retry", retryCount),
			)
			if retryCount >= s.config.MaxRetries {
				return fmt.Errorf("max retries exceeded while monitoring pool")
			}
			continue
		}
		retryCount = 0

		// Проверяем изменения в пуле
		if s.hasSignificantChanges(&initialState, &currentPool.State) {
			s.logger.Info("detected significant pool changes",
				zap.Uint64("oldBaseReserve", initialState.BaseReserve),
				zap.Uint64("newBaseReserve", currentPool.State.BaseReserve),
				zap.Uint64("oldQuoteReserve", initialState.QuoteReserve),
				zap.Uint64("newQuoteReserve", currentPool.State.QuoteReserve),
			)

			if !IsPoolActive(currentPool) {
				return fmt.Errorf("pool became inactive")
			}

			initialState = currentPool.State
		}
	}

	return nil
}

// Вспомогательный метод для ожидания подтверждения транзакции
func (s *Sniper) waitForConfirmation(ctx context.Context, signature string) (string, error) {
	sig, err := solana.SignatureFromBase58(signature)
	if err != nil {
		return "", fmt.Errorf("invalid signature: %w", err)
	}

	for i := 0; i < 30; i++ { // максимум 30 попыток
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
			status, err := s.client.client.GetSignatureStatuses(ctx, sig)
			if err != nil {
				return "", fmt.Errorf("failed to get status: %w", err)
			}
			if status != nil && len(status.Value) > 0 && status.Value[0] != nil {
				if status.Value[0].Err != nil {
					return "failed", fmt.Errorf("transaction failed: %v", status.Value[0].Err)
				}
				return string(status.Value[0].ConfirmationStatus), nil // Используем напрямую без разыменования
			}
			time.Sleep(500 * time.Millisecond)
		}
	}
	return "", fmt.Errorf("confirmation timeout")
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
	if s.config.MaxSlippageBps == 0 || s.config.MaxSlippageBps > 10000 { // 10000 = 100%
		return fmt.Errorf("invalid slippage: must be between 0 and 10000")
	}

	if s.config.MinAmountSOL <= 0 || s.config.MaxAmountSOL <= 0 {
		return fmt.Errorf("invalid amount parameters")
	}

	if s.config.MaxAmountSOL < s.config.MinAmountSOL {
		return fmt.Errorf("maxAmount cannot be less than minAmount")
	}

	// Проверяем mint addresses
	if s.config.BaseMint.IsZero() || s.config.QuoteMint.IsZero() {
		return fmt.Errorf("invalid mint addresses")
	}

	// Проверяем наличие достаточного баланса
	balance, err := s.client.client.GetBalance(
		context.Background(),
		s.client.privateKey.PublicKey(),
		solanarpc.CommitmentConfirmed,
	)
	if err != nil {
		return fmt.Errorf("failed to get wallet balance: %w", err)
	}

	if balance/solana.LAMPORTS_PER_SOL < s.config.MinAmountSOL {
		return fmt.Errorf("insufficient balance")
	}

	// Проверяем параметры мониторинга
	if s.config.MonitorInterval < time.Second {
		return fmt.Errorf("monitor interval too small")
	}

	if s.config.MaxRetries < 1 {
		return fmt.Errorf("invalid max retries value")
	}

	return nil
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
