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

	// 2. Получение информации о пуле
	pool, err := s.client.GetPool(ctx, s.config.BaseMint, s.config.QuoteMint)
	if err != nil {
		return fmt.Errorf("failed to get pool: %w", err)
	}

	// 3. Проверка состояния пула и ликвидности
	if err := s.client.checkPoolLiquidity(ctx, pool); err != nil {
		return fmt.Errorf("pool liquidity check failed: %w", err)
	}

	// 4. Подготовка токен аккаунтов
	accounts, err := s.client.ensureTokenAccounts(ctx, pool.BaseMint, pool.QuoteMint)
	if err != nil {
		return fmt.Errorf("failed to prepare token accounts: %w", err)
	}

	// 5. Расчет параметров свапа с учетом слиппажа
	amounts := CalculateSwapAmounts(pool, s.config.MinAmountSOL, s.config.MaxSlippageBps)

	// 6. Подготовка параметров свапа
	swapParams := &SwapParams{
		UserWallet:              s.client.GetPublicKey(),
		AmountIn:                amounts.AmountIn,
		MinAmountOut:            amounts.MinAmountOut,
		Pool:                    pool,
		SourceTokenAccount:      accounts.SourceATA,
		DestinationTokenAccount: accounts.DestinationATA,
		PriorityFeeLamports:     s.config.PriorityFee,
		Direction:               SwapDirectionIn,
		SlippageBps:             s.config.MaxSlippageBps,
	}

	// 7. Подготовка транзакции
	if err := s.client.prepareSwap(ctx, swapParams); err != nil {
		return fmt.Errorf("failed to prepare swap: %w", err)
	}

	// 8. Исполнение свапа
	signature, err := s.executeSwapWithRetry(ctx, swapParams)
	if err != nil {
		return fmt.Errorf("swap execution failed: %w", err)
	}

	s.logger.Info("snipe executed successfully",
		zap.String("signature", signature.String()),
		zap.Uint64("amount_in", amounts.AmountIn),
		zap.Uint64("min_amount_out", amounts.MinAmountOut))

	return nil
}

// MonitorPool отслеживает изменения в пуле
func (s *Sniper) MonitorPool(ctx context.Context) error {
	ticker := time.NewTicker(s.config.MonitorInterval)
	defer ticker.Stop()

	var lastState *PoolState
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			pool, err := s.client.GetPool(ctx, s.config.BaseMint, s.config.QuoteMint)
			if err != nil {
				s.logger.Warn("failed to get pool state", zap.Error(err))
				continue
			}

			if lastState != nil && s.hasSignificantChanges(lastState, &pool.State) {
				if err := s.ExecuteSnipe(ctx); err != nil {
					s.logger.Error("snipe execution failed", zap.Error(err))
				}
			}

			lastState = &pool.State
		}
	}
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

func (s *Sniper) executeSwapWithRetry(ctx context.Context, params *SwapParams) (solana.Signature, error) {
	var lastErr error
	for attempt := 0; attempt < s.config.MaxRetries; attempt++ {
		signature, err := s.client.Swap(ctx, params)
		if err == nil {
			return signature, nil
		}
		lastErr = err
		time.Sleep(time.Second * time.Duration(1<<uint(attempt)))
	}
	return solana.Signature{}, fmt.Errorf("max retries exceeded: %w", lastErr)
}

func (s *Sniper) hasSignificantChanges(old, new *PoolState) bool {
	if old.Status != new.Status {
		return true
	}

	baseChange := math.Abs(float64(new.BaseReserve-old.BaseReserve)) / float64(old.BaseReserve)
	quoteChange := math.Abs(float64(new.QuoteReserve-old.QuoteReserve)) / float64(old.QuoteReserve)

	const significantChangeThreshold = 0.01 // 1%
	return baseChange > significantChangeThreshold || quoteChange > significantChangeThreshold
}
