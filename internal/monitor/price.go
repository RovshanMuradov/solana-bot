// internal/monitor/price.go
package monitor

import (
	"context"
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

// Start запускает мониторинг в собственной горутине и корректно выходит при Stop.
func (pm *PriceMonitor) Start() {
	pm.logger.Info("PriceMonitor: start", zap.String("token", pm.tokenMint))
	ticker := time.NewTicker(pm.interval)
	defer ticker.Stop()

	// первая итерация сразу
	pm.updatePrice()

	for {
		select {
		case <-pm.ctx.Done():
			pm.logger.Info("PriceMonitor: context done, exiting loop")
			return
		case <-ticker.C:
			pm.updatePrice()
		}
	}
}

// Stop отменяет контекст мониторинга
func (pm *PriceMonitor) Stop() {
	pm.cancel()
}

// updatePrice использует разделенный локальный контекст с таймаутом для RPC
func (pm *PriceMonitor) updatePrice() {
	// Создаем новый контекст для получения цены
	cctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	price, err := pm.dex.GetTokenPrice(cctx, pm.tokenMint)
	if err != nil {
		pm.logger.Error("GetTokenPrice error", zap.Error(err))
		return
	}
	// Вычисляем изменение цены и вызываем callback
	pm.callback(price, pm.initialPrice, ((price-pm.initialPrice)/pm.initialPrice)*100, pm.tokenAmount)
}

// SetCallback устанавливает функцию обратного вызова для обновлений цены.
func (pm *PriceMonitor) SetCallback(callback PriceUpdateCallback) {
	pm.callback = callback
}
