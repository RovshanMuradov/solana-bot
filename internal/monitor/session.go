// internal/monitor/session.go
package monitor

import (
	"context"
	"fmt"
	"github.com/rovshanmuradov/solana-bot/internal/dex/model"
	"github.com/rovshanmuradov/solana-bot/internal/task"
	"math"
	"sync"
	"time"

	"github.com/rovshanmuradov/solana-bot/internal/dex"
	"go.uber.org/zap"
)

// SessionConfig contains configuration for a monitoring session
type SessionConfig struct {
	Task            *task.Task    // ✅ ссылка на исходную задачу
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
	t := ms.config.Task // 👈 просто для краткости

	ms.logger.Info("Preparing monitoring session",
		zap.String("token", t.TokenMint),
		zap.Float64("initial_investment_sol", t.AmountSol))

	initialPrice := ms.config.InitialPrice
	initialTokens := t.AutosellAmount

	if initialPrice == 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		initialPrice, _ = ms.config.DEX.GetTokenPrice(ctx, t.TokenMint)
		cancel()
	}
	if initialTokens == 0 && ms.config.TokenBalance == 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		bal, err := ms.config.DEX.GetTokenBalance(ctx, t.TokenMint)
		cancel()
		if err == nil {
			ms.config.TokenBalance = bal
			initialTokens = float64(bal) / 1e6
		}
	}

	ms.logger.Info("Monitor start",
		zap.String("token", t.TokenMint),
		zap.Float64("initial_price", initialPrice),
		zap.Float64("initial_tokens", initialTokens),
		zap.Uint64("initial_tokens_raw", ms.config.TokenBalance))

	ms.config.InitialPrice = initialPrice
	t.AutosellAmount = initialTokens

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

	// Create an input handler
	ms.inputHandler = NewInputHandler(ms.ctx, ms.logger.Named("input"))

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
	ms.logger.Debug("Stopping monitoring session...") // Добавим лог

	// Stop the input handler first (usually quick)
	if ms.inputHandler != nil {
		ms.inputHandler.Stop()
		ms.logger.Debug("Input handler stopped.")
	}

	// Stop the price monitor (cancels its context)
	if ms.priceMonitor != nil {
		ms.priceMonitor.Stop()
		ms.logger.Debug("Price monitor stop signal sent.")
	}

	// Cancel the main session context (if not already done by monitor stop)
	if ms.cancel != nil {
		ms.cancel()
		ms.logger.Debug("Main session context cancelled.")
	}

	// Ждем, пока горутина, запущенная в Start для priceMonitor.Start(),
	// действительно завершится после отмены контекста.
	doneChan := make(chan struct{})
	go func() {
		ms.wg.Wait() // Ждем завершения всех горутин в группе (сейчас там только монитор)
		close(doneChan)
	}()

	// Даем некоторое время на завершение, но не блокируем навсегда
	select {
	case <-doneChan:
		ms.logger.Debug("Monitoring goroutine finished gracefully.")
	case <-time.After(5 * time.Second): // Таймаут ожидания
		ms.logger.Warn("Timeout waiting for monitoring goroutine to finish.")
	}
	ms.logger.Debug("Monitoring session Stop completed.")
}

// onPriceUpdate вызывается при обновлении цены токена.
//
// Метод координирует получение актуальной информации о балансе токена,
// расчет прибыли/убытков и вывод этой информации в консоль.
func (ms *MonitoringSession) onPriceUpdate(currentPrice, initialPrice, percentChange, tokenAmount float64) {
	ctx, cancel := context.WithTimeout(ms.ctx, 5*time.Second)
	defer cancel()

	// Шаг 1: Обновляем баланс и рассчитываем PnL
	updatedBalance, pnlData, err := ms.updateBalanceAndCalculatePnL(ctx, tokenAmount)
	if err != nil {
		return // Функция updateBalanceAndCalculatePnL уже логирует ошибку
	}

	// Шаг 2: Отображаем информацию
	ms.displayMonitorInfo(currentPrice, initialPrice, percentChange, updatedBalance, pnlData)
}

// updateBalanceAndCalculatePnL обновляет баланс токенов и рассчитывает PnL.
//
// Функция запрашивает актуальный баланс токенов, обновляет его в конфигурации
// и рассчитывает текущую прибыль/убыток на основе этой информации.
func (ms *MonitoringSession) updateBalanceAndCalculatePnL(ctx context.Context, currentAmount float64) (float64, *model.PnLResult, error) {
	t := ms.config.Task

	// Шаг 1: Пробуем получить актуальный баланс токена
	tokenBalanceRaw, err := ms.config.DEX.GetTokenBalance(ctx, t.TokenMint)
	if err != nil {
		ms.logger.Error("Failed to get token balance", zap.Error(err))
		return currentAmount, nil, err
	}

	// Шаг 2: Если получили — обновим локальную переменную
	updatedBalance := currentAmount
	if tokenBalanceRaw > 0 {
		newBalance := float64(tokenBalanceRaw) / 1e6
		if math.Abs(newBalance-currentAmount) > 0.000001 && newBalance > 0 {
			ms.logger.Debug("Token balance changed",
				zap.Float64("old_balance", currentAmount),
				zap.Float64("new_balance", newBalance))
			ms.config.TokenBalance = tokenBalanceRaw
			updatedBalance = newBalance
		}
	}

	// Шаг 3: Получаем PnL калькулятор
	calculator, err := GetCalculator(ms.config.DEX, ms.logger)
	if err != nil {
		ms.logger.Error("Failed to get calculator for DEX", zap.Error(err))
		fmt.Printf("\nError: Cannot calculate PnL for %s\n", ms.config.DEX.GetName())
		return updatedBalance, nil, err
	}

	// Шаг 4: Считаем PnL по текущему балансу, но исходной цене покупки
	pnlData, err := calculator.CalculatePnL(ctx, updatedBalance, t.AmountSol)
	if err != nil {
		ms.logger.Error("Failed to calculate PnL", zap.Error(err))
		fmt.Printf("\nError calculating PnL: %v\n", err)
		return updatedBalance, nil, err
	}

	return updatedBalance, pnlData, nil
}

// displayMonitorInfo форматирует и выводит информацию о мониторинге в консоль.
func (ms *MonitoringSession) displayMonitorInfo(currentPrice, initialPrice, percentChange, tokenBalance float64, pnlData *model.PnLResult) {
	t := ms.config.Task
	// Если сессия уже отменена — сразу выходим
	select {
	case <-ms.ctx.Done():
		return
	default:
	}
	// pnlData уже имеет правильный тип *PnLData
	pnl := pnlData

	// Форматирование процента изменения цены
	changeStr := fmt.Sprintf("%.2f%%", percentChange)
	if percentChange > 0 {
		changeStr = "\033[32m+" + changeStr + "\033[0m" // Зеленый для роста
	} else if percentChange < 0 {
		changeStr = "\033[31m" + changeStr + "\033[0m" // Красный для падения
	}

	// Форматирование PnL
	pnlStr := fmt.Sprintf("%.8f SOL (%.2f%%)", pnl.NetPnL, pnl.PnLPercentage)
	if pnl.NetPnL > 0 {
		pnlStr = "\033[32m+" + pnlStr + "\033[0m" // Зеленый для прибыли
	} else if pnl.NetPnL < 0 {
		pnlStr = "\033[31m" + pnlStr + "\033[0m" // Красный для убытка
	}

	// Вывод информации в консоль
	fmt.Println("\n╔════════════════ TOKEN MONITOR ════════════════╗")
	fmt.Printf("║ Token: %-38s ║\n", shortenAddress(t.TokenMint))
	fmt.Println("╟───────────────────────────────────────────────╢")
	fmt.Printf("║ Current Price:       %-14.8f SOL ║\n", currentPrice)
	fmt.Printf("║ Initial Price:       %-14.8f SOL ║\n", initialPrice)
	fmt.Printf("║ Price Change:        %-25s ║\n", changeStr)
	fmt.Printf("║ Tokens Owned:        %-14.6f      ║\n", tokenBalance)
	fmt.Println("╟───────────────────────────────────────────────╢")
	fmt.Printf("║ Sold (Estimate):     %-14.8f SOL ║\n", pnl.SellEstimate)
	fmt.Printf("║ Invested:            %-14.8f SOL ║\n", pnl.InitialInvestment)
	fmt.Printf("║ P&L:                 %-25s ║\n", pnlStr)
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
	t := ms.config.Task

	fmt.Println("\nSelling tokens...")
	ms.cancel()
	ms.inputHandler.Stop()
	ms.priceMonitor.Stop()

	ms.logger.Info("Executing sell operation",
		zap.String("token_mint", t.TokenMint),
		zap.Float64("autosell_amount", t.AutosellAmount),
		zap.Float64("slippage_percent", t.SlippagePercent),
		zap.String("priority_fee", t.PriorityFeeSol),
		zap.Uint32("compute_units", t.ComputeUnits))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := ms.config.DEX.SellPercentTokens(
		ctx,
		t.TokenMint,
		t.AutosellAmount,
		t.SlippagePercent,
		t.PriorityFeeSol,
		t.ComputeUnits,
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
