// internal/dex/raydium/sniper.go
package raydium

import (
	"context"
	"fmt"
	"math"

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

	if err := s.validateConfig(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	// Получаем пул напрямую через GetPool
	pool, err := s.client.GetPool(ctx, s.config.BaseMint, s.config.QuoteMint)
	if err != nil {
		return fmt.Errorf("failed to get pool: %w", err)
	}

	direction, err := s.client.DetermineSwapDirection(pool, s.config.BaseMint, s.config.QuoteMint)
	if err != nil {
		return fmt.Errorf("failed to determine swap direction: %w", err)
	}

	// Подготовка параметров свапа
	accounts, err := s.client.ensureTokenAccounts(ctx, pool.BaseMint, pool.QuoteMint)
	if err != nil {
		return fmt.Errorf("failed to prepare token accounts: %w", err)
	}

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

	// Используем RetrySwap из client вместо executeSwapWithRetry
	result, err := s.client.RetrySwap(ctx, params)
	if err != nil {
		return fmt.Errorf("swap execution failed: %w", err)
	}

	s.logger.Info("snipe executed successfully",
		zap.String("signature", result.Signature.String()),
		zap.Uint64("amount_in", params.AmountIn))

	return nil
}

// MonitorPool отслеживает изменения цены в SOL
func (s *Sniper) MonitorPool(ctx context.Context) error {
	s.logger.Info("starting price monitoring")

	monitor, err := s.client.api.StartPriceMonitoring(ctx, &Pool{
		ID: solana.MustPublicKeyFromBase58(s.config.BaseMint.String()),
	})
	if err != nil {
		return fmt.Errorf("failed to start price monitoring: %w", err)
	}
	defer monitor.Stop()

	var lastUpdate PriceUpdate
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case update := <-monitor.GetPriceUpdates():
			if s.hasSignificantPriceChange(lastUpdate, update) {
				s.logger.Info("significant price change detected",
					zap.Float64("price_in_sol", update.PriceInSol),
					zap.Float64("change_percent", update.PriceChangePerc))

				if s.shouldExecuteSnipe(update) {
					if err := s.ExecuteSnipe(ctx); err != nil {
						s.logger.Error("snipe execution failed", zap.Error(err))
					}
				}
			}
			lastUpdate = update
		}
	}
}

// hasSignificantPriceChange определяет значимые изменения цены
func (s *Sniper) hasSignificantPriceChange(old, new PriceUpdate) bool {
	if old.PriceInSol == 0 {
		return false
	}

	priceChange := math.Abs(new.PriceChangePerc)
	return priceChange > 1.0 // Значимое изменение > 1%
}

// shouldExecuteSnipe проверяет условия для выполнения свапа
func (s *Sniper) shouldExecuteSnipe(update PriceUpdate) bool {
	return update.PriceChangePerc < 0 && // Цена снижается
		update.LiquidityInSol > float64(s.config.MinAmountSOL*2) // Достаточная ликвидность
}

// validateConfig проверяет базовую конфигурацию
func (s *Sniper) validateConfig() error {
	if s.config.BaseMint.IsZero() || s.config.QuoteMint.IsZero() {
		return fmt.Errorf("invalid mint addresses")
	}

	if s.config.MinAmountSOL == 0 {
		return fmt.Errorf("invalid minimum amount")
	}

	return nil
}
