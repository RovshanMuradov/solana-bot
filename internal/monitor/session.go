// internal/monitor/session.go
package monitor

import (
	"context"
	"errors"
	"math"
	"sync"
	"time"

	"github.com/gagliardetto/solana-go"
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
}

// NewMonitoringSession создает новую сессию мониторинга.
func NewMonitoringSession(parentCtx context.Context, config *SessionConfig) *MonitoringSession {
	ctx, cancel := context.WithCancel(parentCtx)
	return &MonitoringSession{
		config:       config,
		logger:       config.Logger,
		ctx:          ctx,
		cancel:       cancel,
		priceUpdates: make(chan PriceUpdate),
		errChan:      make(chan error),
	}
}

// Start запускает сессию мониторинга.
func (ms *MonitoringSession) Start() error {
	t := ms.config.Task // 👈 просто для краткости

	ms.logger.Info("Preparing monitoring session",
		zap.String("token", t.TokenMint),
		zap.Float64("initial_investment_sol", t.AmountSol))

	initialPrice := ms.config.InitialPrice

	// 1. Get token balance through the DEX adapter
	ctx, cancel := context.WithTimeout(ms.ctx, 5*time.Second)
	raw, err := ms.config.DEX.GetTokenBalance(ctx, t.TokenMint)
	if err != nil {
		ms.logger.Error("Failed to fetch token balance", zap.Error(err))
	} else {
		ms.config.TokenBalance = raw
	}

	var minTokenDecimals uint8 = 6 // DefaultTokenDecimals

	// 2. Calculate actual token amount with correct decimals through DEX
	initialTokens := 0.0
	if ms.config.TokenBalance > 0 {
		// Get token decimals from the blockchain
		tokenPK := solana.MustPublicKeyFromBase58(t.TokenMint)
		dec, err := getTokenDecimals(ctx, ms.config.DEX, tokenPK, minTokenDecimals)
		if err != nil {
			ms.logger.Warn("Failed to get token decimals, using default", zap.Error(err), zap.Uint8("default_decimals", minTokenDecimals))
			dec = minTokenDecimals
		}

		initialTokens = float64(ms.config.TokenBalance) / math.Pow10(int(dec))
	}
	cancel()

	// 3. Calculate real purchase price from SOL spent / tokens received
	if initialTokens > 0 {
		initialPrice = t.AmountSol / initialTokens
	}

	ms.logger.Info("Monitor start",
		zap.String("token", t.TokenMint),
		zap.Float64("initial_price", initialPrice),
		zap.Float64("initial_tokens", initialTokens),
		zap.Uint64("initial_tokens_raw", ms.config.TokenBalance))

	ms.config.InitialPrice = initialPrice

	// Only update AutosellAmount if we've determined a valid token amount
	if initialTokens > 0 {
		t.AutosellAmount = initialTokens
	}

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
				ms.logger.Warn("Error channel blocked, dropping error", zap.Error(err))
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

// getTokenDecimals обертка для получения десятичных знаков токена от DEX
func getTokenDecimals(ctx context.Context, dexAdapter dex.DEX, tokenMint solana.PublicKey, defaultDecimals uint8) (uint8, error) {
	// Проверяем тип DEX
	switch d := dexAdapter.(type) {
	case interface {
		getTokenDecimals(context.Context, solana.PublicKey, uint8) uint8
	}:
		// Используем метод, если он есть
		return d.getTokenDecimals(ctx, tokenMint, defaultDecimals), nil
	default:
		return defaultDecimals, errors.New("DEX adapter does not support getTokenDecimals")
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
		ms.logger.Error("Failed to get token balance", zap.Error(err))
		return currentAmount, err
	}

	// Если получили — обновим локальную переменную
	updatedBalance := currentAmount
	if tokenBalanceRaw > 0 {
		// Получаем правильную десятичную точность
		tokenPK := solana.MustPublicKeyFromBase58(t.TokenMint)
		var defaultDecimals uint8 = 6
		dec, err := getTokenDecimals(ctx, ms.config.DEX, tokenPK, defaultDecimals)
		if err != nil {
			ms.logger.Warn("Using default decimals for balance update", zap.Error(err))
			dec = defaultDecimals
		}

		newBalance := float64(tokenBalanceRaw) / math.Pow10(int(dec))
		if math.Abs(newBalance-currentAmount) > 0.000001 && newBalance > 0 {
			ms.logger.Debug("Token balance changed",
				zap.Float64("old_balance", currentAmount),
				zap.Float64("new_balance", newBalance),
				zap.Uint8("decimals", dec))
			ms.config.TokenBalance = tokenBalanceRaw
			updatedBalance = newBalance
		}
	}

	return updatedBalance, nil
}
