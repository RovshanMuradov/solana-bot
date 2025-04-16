// internal/monitor/price.go
package monitor

import (
	"context"
	"math"
	"time"

	"github.com/rovshanmuradov/solana-bot/internal/dex"
	"go.uber.org/zap"
)

// PriceUpdateCallback - функция обратного вызова, вызываемая при обновлении цены токена.
type PriceUpdateCallback func(currentPriceSol float64, initialPriceSol float64, percentChange float64, tokenAmount float64)

// PriceMonitor отслеживает изменения цены токена.
type PriceMonitor struct {
	dex           dex.DEX             // DEX interface for price retrieval
	interval      time.Duration       // Interval between price checks
	initialPrice  float64             // Initial token price when monitoring started
	tokenAmount   float64             // Amount of tokens purchased
	tokenMint     string              // Token mint address
	initialAmount float64             // Initial SOL amount spent
	logger        *zap.Logger         // Logger
	callback      PriceUpdateCallback // Callback for price updates
	ctx           context.Context     // Context for cancellation
	cancel        context.CancelFunc  // Cancel function
}

// NewPriceMonitor создает новый монитор цены токена.
func NewPriceMonitor(dex dex.DEX, tokenMint string, initialPrice float64,
	tokenAmount float64, initialAmount float64,
	interval time.Duration, logger *zap.Logger,
	callback PriceUpdateCallback) *PriceMonitor {
	ctx, cancel := context.WithCancel(context.Background())
	return &PriceMonitor{
		dex:           dex,
		interval:      interval,
		initialPrice:  initialPrice,
		tokenAmount:   tokenAmount,
		tokenMint:     tokenMint,
		initialAmount: initialAmount,
		logger:        logger,
		callback:      callback,
		ctx:           ctx,
		cancel:        cancel,
	}
}

// Start запускает процесс мониторинга цены.
func (pm *PriceMonitor) Start() {
	pm.logger.Info("Starting price monitor",
		zap.String("token_mint", pm.tokenMint),
		zap.Float64("initial_price", pm.initialPrice),
		zap.Duration("interval", pm.interval))

	// Run the first update immediately
	pm.updatePrice()

	// Start the ticker for periodic updates
	ticker := time.NewTicker(pm.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			pm.updatePrice()
		case <-pm.ctx.Done():
			pm.logger.Debug("Price monitor stopped")
			return
		}
	}
}

// Stop останавливает мониторинг цены.
func (pm *PriceMonitor) Stop() {
	if pm.cancel != nil {
		pm.cancel()
	}
}

// updatePrice получает текущую цену токена и вызывает функцию обратного вызова.
func (pm *PriceMonitor) updatePrice() {
	// Сначала проверяем, не отменен ли контекст
	if pm.ctx.Err() != nil {
		pm.logger.Debug("Price monitor stopping, skipping price update")
		return
	}

	ctx, cancel := context.WithTimeout(pm.ctx, 10*time.Second)
	defer cancel()

	// Получаем текущую цену токена
	currentPrice, err := pm.dex.GetTokenPrice(ctx, pm.tokenMint)
	if err != nil {
		pm.logger.Error("Failed to get token price", zap.Error(err))
		return
	}

	// Рассчитываем простое процентное изменение цены (отличается от PnL percentage)
	// Это чистое изменение рыночной цены, без учета комиссий и других факторов
	percentChange := 0.0
	if pm.initialPrice > 0 {
		percentChange = ((currentPrice - pm.initialPrice) / pm.initialPrice) * 100
	}

	// Округляем до 2 десятичных знаков для удобства отображения
	percentChange = math.Floor(percentChange*100) / 100

	// Вызываем обратный вызов с информацией о цене
	if pm.callback != nil {
		pm.callback(currentPrice, pm.initialPrice, percentChange, pm.tokenAmount)
	}
}

// SetCallback устанавливает функцию обратного вызова для обновлений цены.
func (pm *PriceMonitor) SetCallback(callback PriceUpdateCallback) {
	pm.callback = callback
}
