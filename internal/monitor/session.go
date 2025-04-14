// internal/monitor/session.go
package monitor

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/rovshanmuradov/solana-bot/internal/dex"
	"go.uber.org/zap"
)

// SessionConfig contains configuration for a monitoring session
type SessionConfig struct {
	TokenMint       string        // Token mint address
	TokenAmount     float64       // Human-readable amount of tokens purchased
	TokenBalance    uint64        // Raw token balance in smallest units
	InitialAmount   float64       // Initial SOL amount spent
	InitialPrice    float64       // Initial token price
	MonitorInterval time.Duration // Interval for price updates
	DEX             dex.DEX       // DEX interface
	Logger          *zap.Logger   // Logger

	// Transaction parameters from the original task
	SlippagePercent float64 // Slippage percentage for transactions
	PriorityFee     string  // Priority fee for transactions
	ComputeUnits    uint32  // Compute units for transactions
}

// MonitoringSession представляет сессию мониторинга токенов для операций на DEX.
type MonitoringSession struct {
	config       *SessionConfig
	priceMonitor *PriceMonitor
	inputHandler *InputHandler
	wg           sync.WaitGroup
	ctx          context.Context
	cancel       context.CancelFunc
	logger       *zap.Logger
}

// NewMonitoringSession создает новую сессию мониторинга.
func NewMonitoringSession(config *SessionConfig) *MonitoringSession {
	ctx, cancel := context.WithCancel(context.Background())
	return &MonitoringSession{
		config: config,
		logger: config.Logger,
		ctx:    ctx,
		cancel: cancel,
	}
}

// Start запускает сессию мониторинга.
func (ms *MonitoringSession) Start() error {
	// Create a price monitor
	ms.priceMonitor = NewPriceMonitor(
		ms.config.DEX,
		ms.config.TokenMint,
		ms.config.InitialPrice,
		ms.config.TokenAmount,
		ms.config.InitialAmount,
		ms.config.MonitorInterval,
		ms.logger.Named("price"),
		ms.onPriceUpdate,
	)

	// Create an input handler
	ms.inputHandler = NewInputHandler(ms.logger.Named("input"))

	// Register commands
	ms.inputHandler.RegisterCommand("", ms.onEnterPressed) // Empty command (Enter key)
	ms.inputHandler.RegisterCommand("q", ms.onExitCommand)
	ms.inputHandler.RegisterCommand("exit", ms.onExitCommand)

	// Start the components
	ms.wg.Add(1)
	go func() {
		defer ms.wg.Done()
		ms.priceMonitor.Start()
	}()

	ms.inputHandler.Start()

	// Показать простое сообщение о начале мониторинга
	fmt.Println("\nMonitoring started. Press Enter to sell tokens or 'q' to exit.")
	return nil
}

// Wait ожидает завершения сессии мониторинга.
func (ms *MonitoringSession) Wait() error {
	ms.wg.Wait()
	return nil
}

// Stop останавливает сессию мониторинга.
func (ms *MonitoringSession) Stop() {
	// Stop the input handler
	if ms.inputHandler != nil {
		ms.inputHandler.Stop()
	}

	// Stop the price monitor
	if ms.priceMonitor != nil {
		ms.priceMonitor.Stop()
	}

	// Cancel the context
	if ms.cancel != nil {
		ms.cancel()
	}
}

// onPriceUpdate вызывается при обновлении цены токена.
//
// Метод получает актуальную информацию о цене токена и его балансе,
// вычисляет прибыль/убытки (PnL) и выводит эту информацию в консоль.
func (ms *MonitoringSession) onPriceUpdate(currentPrice, initialPrice, percentChange, tokenAmount float64) {
	// 1. Создаем контекст с таймаутом для выполнения операций
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 2. Проверяем, не изменился ли баланс токенов
	var updatedBalance float64 = tokenAmount

	tokenBalanceRaw, err := ms.config.DEX.GetTokenBalance(ctx, ms.config.TokenMint)
	if err == nil && tokenBalanceRaw > 0 {
		// Если удалось получить актуальный баланс
		updatedBalance = float64(tokenBalanceRaw) / 1e6
		if math.Abs(updatedBalance-tokenAmount) > 0.000001 && updatedBalance > 0 {
			// Если баланс изменился, обновляем TokenAmount и TokenBalance в конфигурации
			ms.logger.Debug("Token balance changed",
				zap.Float64("old_balance", tokenAmount),
				zap.Float64("new_balance", updatedBalance))

			// Обновляем значения в конфигурации
			ms.config.TokenAmount = updatedBalance
			ms.config.TokenBalance = tokenBalanceRaw
		}
	}

	// 3. Получаем данные о PnL с использованием специализированного метода DEX
	pnlData, err := ms.config.DEX.CalculateBondingCurvePnL(ctx, updatedBalance, ms.config.InitialAmount)
	if err != nil {
		ms.logger.Debug("Failed to calculate PnL using DEX method",
			zap.Error(err))
	}

	// 4. Форматируем вывод информации
	fmt.Println("\n╔════════════════ TOKEN MONITOR ════════════════╗")
	fmt.Printf("║ Token: %-38s ║\n", shortenAddress(ms.config.TokenMint))
	fmt.Println("╟───────────────────────────────────────────────╢")
	fmt.Printf("║ Current Price: %-9.8f SOL %15s ║\n", currentPrice, "")
	fmt.Printf("║ Initial Price: %-9.8f SOL %15s ║\n", initialPrice, "")

	// Форматирование изменения цены с цветом (зеленый для положительного, красный для отрицательного)
	changeStr := fmt.Sprintf("%.2f%%", percentChange)
	if percentChange > 0 {
		changeStr = "\033[32m+" + changeStr + "\033[0m" // Зеленый цвет для положительного изменения
	} else if percentChange < 0 {
		changeStr = "\033[31m" + changeStr + "\033[0m" // Красный цвет для отрицательного изменения
	}
	fmt.Printf("║ Price Change: %-40s ║\n", changeStr)

	fmt.Printf("║ Tokens Owned: %-9.6f %20s ║\n", updatedBalance, "")
	fmt.Println("╟───────────────────────────────────────────────╢")
	fmt.Printf("║ Theoretical Value: %-9.8f SOL %11s ║\n", pnlData.TheoreticalValue, "")
	fmt.Printf("║ Sell Estimate:     %-9.8f SOL %11s ║\n", pnlData.SellEstimate, "")
	fmt.Printf("║ Initial Investment: %-9.8f SOL %10s ║\n", pnlData.InitialInvestment, "")

	// Форматирование PnL с цветом
	pnlStr := fmt.Sprintf("%.8f SOL (%.2f%%)", pnlData.NetPnL, pnlData.PnLPercentage)
	if pnlData.NetPnL > 0 {
		pnlStr = "\033[32m+" + pnlStr + "\033[0m" // Зеленый для прибыли
	} else if pnlData.NetPnL < 0 {
		pnlStr = "\033[31m" + pnlStr + "\033[0m" // Красный для убытка
	}
	fmt.Printf("║ P&L: %-49s ║\n", pnlStr)

	fmt.Println("╚═══════════════════════════════════════════════╝")
	fmt.Println("Press Enter to sell tokens, 'q' to exit without selling")
}

// shortenAddress сокращает длинный адрес токена для лучшего отображения
func shortenAddress(address string) string {
	if len(address) <= 20 {
		return address
	}
	return address[:8] + "..." + address[len(address)-8:]
}

// onEnterPressed вызывается при нажатии клавиши Enter.
func (ms *MonitoringSession) onEnterPressed(_ string) error {
	fmt.Println("\nSelling tokens...")

	// Останавливаем сессию мониторинга
	ms.Stop()

	// Процент токенов для продажи (99%)
	percentToSell := 99.0

	ms.logger.Info("Executing sell operation",
		zap.String("token_mint", ms.config.TokenMint),
		zap.Float64("percent_to_sell", percentToSell),
		zap.Float64("slippage_percent", ms.config.SlippagePercent),
		zap.String("priority_fee", ms.config.PriorityFee),
		zap.Uint32("compute_units", ms.config.ComputeUnits))

	// Продаем указанный процент токенов
	// SellPercentTokens будет запрашивать актуальный баланс внутри себя
	err := ms.config.DEX.SellPercentTokens(
		context.Background(),
		ms.config.TokenMint,
		percentToSell,
		ms.config.SlippagePercent,
		ms.config.PriorityFee,
		ms.config.ComputeUnits,
	)

	if err != nil {
		fmt.Printf("Error selling tokens: %v\n", err)
		return err
	}

	fmt.Println("Tokens sold successfully!")
	return nil
}

// onExitCommand вызывается при вводе команды выхода.
func (ms *MonitoringSession) onExitCommand(_ string) error {
	fmt.Println("\nExiting monitor mode without selling tokens.")
	ms.Stop()
	return nil
}
