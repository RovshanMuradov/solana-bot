// internal/monitor/session.go
package monitor

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/rovshanmuradov/solana-bot/internal/dex"
	"github.com/rovshanmuradov/solana-bot/internal/task"
	"go.uber.org/zap"
)

// SessionConfig contains configuration for a monitoring session
type SessionConfig struct {
	Task            *task.Task    // ссылка на исходную задачу
	TokenBalance    uint64        // Raw token balance in smallest units
	InitialPrice    float64       // Initial token price
	DEX             dex.DEX       // DEX adapter
	Logger          *zap.Logger   // Logger
	MonitorInterval time.Duration // Интервал обновления цены
}

// MonitoringSession представляет сессию мониторинга токенов для операций на DEX.
type MonitoringSession struct {
	config       *SessionConfig
	priceMonitor *PriceMonitor
	wg           sync.WaitGroup
	ctx          context.Context
	cancel       context.CancelFunc
	logger       *zap.Logger
	priceUpdates chan PriceUpdate
	errChan      chan error

	// Current state for sync access
	currentPrice  float64
	currentTokens float64
	mu            sync.RWMutex
}

// NewMonitoringSession создает новую сессию мониторинга.
func NewMonitoringSession(parentCtx context.Context, config *SessionConfig) *MonitoringSession {
	ctx, cancel := context.WithCancel(parentCtx)

	// Calculate initial token amount
	var minTokenDecimals uint8 = 6
	initialTokens := 0.0
	if config.TokenBalance > 0 {
		initialTokens = float64(config.TokenBalance) / math.Pow10(int(minTokenDecimals))
	}

	return &MonitoringSession{
		config:        config,
		logger:        config.Logger,
		ctx:           ctx,
		cancel:        cancel,
		priceUpdates:  make(chan PriceUpdate, 100), // Buffered channel for price updates
		errChan:       make(chan error, 10),        // Buffered channel for errors
		currentPrice:  config.InitialPrice,         // Initialize with entry price
		currentTokens: initialTokens,               // Initialize with token balance
	}
}

// Start запускает сессию мониторинга.
func (ms *MonitoringSession) Start() error {
	t := ms.config.Task // 👈 просто для краткости

	ms.logger.Info(fmt.Sprintf("📊 Preparing monitoring for %s...%s (%.3f SOL)",
		t.TokenMint[:4],
		t.TokenMint[len(t.TokenMint)-4:],
		t.AmountSol))

	initialPrice := ms.config.InitialPrice

	// 1. Get token balance through the DEX adapter
	ctx, cancel := context.WithTimeout(ms.ctx, 5*time.Second)
	raw, err := ms.config.DEX.GetTokenBalance(ctx, t.TokenMint)
	if err != nil {
		ms.logger.Error("❌ Failed to fetch token balance: " + err.Error())
	} else {
		ms.config.TokenBalance = raw
	}

	var minTokenDecimals uint8 = 6 // DefaultTokenDecimals

	// 2. Calculate actual token amount with correct decimals through DEX
	initialTokens := 0.0
	if ms.config.TokenBalance > 0 {
		// Convert the raw balance to a float with the default decimals
		// In a future update this could be enhanced to query token metadata
		initialTokens = float64(ms.config.TokenBalance) / math.Pow10(int(minTokenDecimals))
		ms.logger.Debug(fmt.Sprintf("🔢 Using default token decimals: %d", minTokenDecimals))
	}
	cancel()

	// 3. Calculate real purchase price from SOL spent / tokens received
	if initialTokens > 0 {
		initialPrice = t.AmountSol / initialTokens
	}

	ms.logger.Info(fmt.Sprintf("🚀 Monitor started: %.6f tokens @ $%.8f each", initialTokens, initialPrice))

	ms.config.InitialPrice = initialPrice

	// Создаем монитор цен
	ms.priceMonitor = NewPriceMonitor(
		ms.ctx,
		ms.config.DEX,
		t.TokenMint,
		initialPrice,
		initialTokens,
		t.AmountSol,
		ms.config.MonitorInterval,
		ms.logger.Named("price"),
		ms.onPriceUpdate,
	)

	// Start the price monitor in a goroutine
	ms.wg.Add(1)
	go func() {
		defer ms.wg.Done()
		ms.priceMonitor.Start()
	}()

	return nil
}

// Wait ожидает завершения сессии мониторинга.
func (ms *MonitoringSession) Wait() error {
	ms.wg.Wait()
	return nil
}

// Stop останавливает сессию мониторинга.
func (ms *MonitoringSession) Stop() {
	ms.logger.Debug("Stopping monitoring session...")

	// Stop the price monitor (cancels its context)
	if ms.priceMonitor != nil {
		ms.priceMonitor.Stop()
		ms.logger.Debug("Price monitor stop signal sent.")
	}

	// Cancel the main session context
	if ms.cancel != nil {
		ms.cancel()
		ms.logger.Debug("Main session context cancelled.")
	}

	// Ждем, пока горутина, запущенная в Start для priceMonitor.Start(),
	// действительно завершится после отмены контекста.
	doneChan := make(chan struct{})
	go func() {
		ms.wg.Wait() // Ждем завершения всех горутин в группе
		close(doneChan)
	}()

	// Даем некоторое время на завершение, но не блокируем навсегда
	select {
	case <-doneChan:
		ms.logger.Debug("Monitoring goroutine finished gracefully.")
	case <-time.After(5 * time.Second): // Таймаут ожидания
		ms.logger.Warn("Timeout waiting for monitoring goroutine to finish.")
	}

	// Закрываем каналы обновлений и ошибок
	close(ms.priceUpdates)
	close(ms.errChan)

	ms.logger.Debug("Monitoring session Stop completed.")
}

// PriceUpdates возвращает канал для получения обновлений цены
func (ms *MonitoringSession) PriceUpdates() <-chan PriceUpdate {
	return ms.priceUpdates
}

// Err возвращает канал для получения ошибок
func (ms *MonitoringSession) Err() <-chan error {
	return ms.errChan
}

// onPriceUpdate вызывается при обновлении цены токена.
//
// Метод отправляет обновленную информацию о цене в канал priceUpdates.
func (ms *MonitoringSession) onPriceUpdate(update PriceUpdate) {
	// Проверяем состояние сессии перед выполнением операций
	select {
	case <-ms.ctx.Done():
		ms.logger.Debug("Session context is done, skipping onPriceUpdate logic")
		return
	default:
		// продолжаем выполнение только если контекст активен
	}

	// Создаем отдельный контекст для операций в этом методе,
	// унаследованный от основного контекста сессии
	ctx, cancel := context.WithTimeout(ms.ctx, 5*time.Second)
	defer cancel()

	// Обновляем баланс токенов, если возможно
	updatedBalance, err := ms.updateTokenBalance(ctx, update.Tokens)
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			// Отправляем ошибку в канал, если это не просто отмена контекста
			select {
			case ms.errChan <- err:
				ms.logger.Debug("Sent balance update error to error channel")
			case <-ms.ctx.Done():
				ms.logger.Debug("Context canceled while trying to send error")
			default:
				ms.logger.Warn("⚠️  Error channel blocked, dropping error: " + err.Error())
			}
		}
		return
	}

	// Создаем обновленный PriceUpdate с актуальным балансом
	updatedPriceUpdate := PriceUpdate{
		Current: update.Current,
		Initial: update.Initial,
		Percent: update.Percent,
		Tokens:  updatedBalance,
	}

	// Update current state for sync access
	ms.mu.Lock()
	ms.currentPrice = update.Current
	ms.currentTokens = updatedBalance
	ms.mu.Unlock()

	// Отправляем обновление в канал
	select {
	case ms.priceUpdates <- updatedPriceUpdate:
		ms.logger.Debug("Sent price update to channel")
	case <-ms.ctx.Done():
		ms.logger.Debug("Context canceled while trying to send price update")
	default:
		ms.logger.Warn("Price update channel blocked, dropping update")
	}
}

// updateTokenBalance обновляет баланс токенов.
//
// Функция запрашивает актуальный баланс токенов и обновляет его в конфигурации.
// Принимает контекст от вызывающей функции.
func (ms *MonitoringSession) updateTokenBalance(ctx context.Context, currentAmount float64) (float64, error) {
	t := ms.config.Task

	// Пробуем получить актуальный баланс токена
	tokenBalanceRaw, err := ms.config.DEX.GetTokenBalance(ctx, t.TokenMint)
	if err != nil {
		ms.logger.Error("❌ Failed to get token balance: " + err.Error())
		return currentAmount, err
	}

	// Если получили — обновим локальную переменную
	updatedBalance := currentAmount
	if tokenBalanceRaw > 0 {
		// Using default decimals - in a real implementation this should
		// ideally come from token metadata
		var defaultDecimals uint8 = 6
		newBalance := float64(tokenBalanceRaw) / math.Pow10(int(defaultDecimals))

		if math.Abs(newBalance-currentAmount) > 0.000001 && newBalance > 0 {
			ms.logger.Debug(fmt.Sprintf("🔄 Token balance changed: %.6f → %.6f", currentAmount, newBalance))
			ms.config.TokenBalance = tokenBalanceRaw
			updatedBalance = newBalance
		}
	}

	return updatedBalance, nil
}

// GetCurrentState returns the current monitoring state
func (ms *MonitoringSession) GetCurrentState() (currentPrice, entryPrice, currentTokens float64, task *task.Task) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	return ms.currentPrice, ms.config.InitialPrice, ms.currentTokens, ms.config.Task
}

// GetTokenMint returns the token mint being monitored
func (ms *MonitoringSession) GetTokenMint() string {
	return ms.config.Task.TokenMint
}

// GetEntryPrice returns the entry price for this position
func (ms *MonitoringSession) GetEntryPrice() float64 {
	return ms.config.InitialPrice
}

// GetCurrentPrice returns the current price (thread-safe)
func (ms *MonitoringSession) GetCurrentPrice() float64 {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	return ms.currentPrice
}

// GetCurrentTokens returns the current token amount (thread-safe)
func (ms *MonitoringSession) GetCurrentTokens() float64 {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	return ms.currentTokens
}

// CalculatePnL calculates current PnL metrics
func (ms *MonitoringSession) CalculatePnL() (pnlPercent, pnlSol float64) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	if ms.config.InitialPrice <= 0 {
		return 0, 0
	}

	// Calculate percentage change
	pnlPercent = ((ms.currentPrice - ms.config.InitialPrice) / ms.config.InitialPrice) * 100

	// Calculate SOL profit/loss
	pnlSol = (ms.currentPrice - ms.config.InitialPrice) * ms.currentTokens

	return pnlPercent, pnlSol
}
