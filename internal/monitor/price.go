// internal/monitor/price.go
package monitor

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/rovshanmuradov/solana-bot/internal/dex"
	"go.uber.org/zap"
)

// PriceUpdate представляет обновление цены токена
type PriceUpdate struct {
	Current  float64 // Текущая цена токена
	Initial  float64 // Начальная цена токена
	Percent  float64 // Процентное изменение цены
	Tokens   float64 // Количество токенов
}

// PriceUpdateCallback - функция обратного вызова, вызываемая при обновлении цены токена.
type PriceUpdateCallback func(update PriceUpdate)

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
	stopped       atomic.Bool         // Флаг остановки, используем atomic для безопасного доступа из разных горутин
}

// NewPriceMonitor создает новый монитор цены токена.
func NewPriceMonitor(parentCtx context.Context, dex dex.DEX, tokenMint string, initialPrice float64,
	tokenAmount float64, initialAmount float64,
	interval time.Duration, logger *zap.Logger,
	callback PriceUpdateCallback) *PriceMonitor {
	ctx, cancel := context.WithCancel(parentCtx)
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
			// Проверяем флаг остановки перед обновлением цены
			if pm.stopped.Load() {
				pm.logger.Debug("PriceMonitor: stopped, skipping price update")
				continue
			}
			pm.updatePrice()
		}
	}
}

// Stop отменяет контекст мониторинга и устанавливает флаг остановки
func (pm *PriceMonitor) Stop() {
	// Сначала устанавливаем флаг остановки, чтобы предотвратить вызов колбека
	pm.stopped.Store(true)
	pm.logger.Debug("PriceMonitor: stop flag set")

	// Затем отменяем контекст
	pm.cancel()
}

// updatePrice использует отдельный контекст с таймаутом для RPC
func (pm *PriceMonitor) updatePrice() {
	// Проверяем флаг остановки перед выполнением операции
	if pm.stopped.Load() {
		pm.logger.Debug("PriceMonitor: already stopped, skipping updatePrice")
		return
	}

	// Проверяем, не отменен ли уже основной контекст
	select {
	case <-pm.ctx.Done():
		pm.logger.Debug("PriceMonitor: context already done, skipping updatePrice")
		return
	default:
		// продолжаем выполнение
	}

	// Создаем новый контекст для получения цены, унаследованный от родительского
	cctx, cancel := context.WithTimeout(pm.ctx, 10*time.Second)
	defer cancel()

	price, err := pm.dex.GetTokenPrice(cctx, pm.tokenMint)
	if err != nil {
		pm.logger.Error("GetTokenPrice error", zap.Error(err))
		return
	}

	// Еще раз проверяем флаг остановки перед вызовом колбека
	if pm.stopped.Load() {
		pm.logger.Debug("PriceMonitor: stopped after price retrieval, skipping callback")
		return
	}

	// Еще раз проверяем, не отменен ли контекст перед вызовом колбека
	select {
	case <-pm.ctx.Done():
		pm.logger.Debug("PriceMonitor: context done after price retrieval, skipping callback")
		return
	default:
		// Вычисляем изменение цены и вызываем callback с объектом PriceUpdate
		percentChange := ((price - pm.initialPrice) / pm.initialPrice) * 100
		update := PriceUpdate{
			Current: price,
			Initial: pm.initialPrice,
			Percent: percentChange,
			Tokens:  pm.tokenAmount,
		}
		pm.callback(update)
	}
}

// SetCallback устанавливает функцию обратного вызова для обновлений цены.
func (pm *PriceMonitor) SetCallback(callback PriceUpdateCallback) {
	pm.callback = callback
}
