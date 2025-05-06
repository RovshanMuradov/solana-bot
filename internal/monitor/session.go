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
	TokenBalance    uint64        // Raw token balance in the smallest units
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
	ms.logger.Info("Preparing monitoring session",
		zap.String("token", ms.config.TokenMint),
		zap.Float64("initial_investment_sol", ms.config.InitialAmount))

	// Получим начальную цену и баланс еще раз для лога, если они не переданы или могут быть неточными
	// Для простоты используем переданные в конфиге значения
	initialPrice := ms.config.InitialPrice
	initialTokens := ms.config.TokenAmount
	// Если цена не была передана, попробуем получить ее
	if initialPrice == 0 {
		priceCtx, priceCancel := context.WithTimeout(context.Background(), 5*time.Second)
		var priceErr error
		initialPrice, priceErr = ms.config.DEX.GetTokenPrice(priceCtx, ms.config.TokenMint)
		if priceErr != nil {
			ms.logger.Warn("Could not get initial price for logging", zap.Error(priceErr))
		}
		priceCancel()
	}
	// Если баланс токенов не был передан (или равен 0), попробуем получить его
	if initialTokens == 0 && ms.config.TokenBalance == 0 {
		balanceCtx, balanceCancel := context.WithTimeout(context.Background(), 5*time.Second)
		balanceRaw, balanceErr := ms.config.DEX.GetTokenBalance(balanceCtx, ms.config.TokenMint)
		if balanceErr == nil {
			ms.config.TokenBalance = balanceRaw
			// Предполагаем 6 знаков после запятой, если не знаем точно
			// TODO: Передать или определить точность токена
			initialTokens = float64(balanceRaw) / 1e6
			ms.config.TokenAmount = initialTokens // Обновляем значение в конфиге
		} else {
			ms.logger.Warn("Could not get initial token balance for logging", zap.Error(balanceErr))
		}
		balanceCancel()
	}

	// Логируем начальные данные
	ms.logger.Info("Monitor start",
		zap.String("token", ms.config.TokenMint),
		zap.Float64("initial_price", initialPrice),
		zap.Float64("initial_tokens", initialTokens),             // Используем количество токенов
		zap.Uint64("initial_tokens_raw", ms.config.TokenBalance)) // И сырой баланс

	// Обновляем InitialPrice в конфиге, если мы его только что получили
	ms.config.InitialPrice = initialPrice

	// Create a price monitor
	ms.priceMonitor = NewPriceMonitor(
		ms.config.DEX,
		ms.config.TokenMint,
		ms.config.InitialPrice,  // Используем актуальную начальную цену
		ms.config.TokenAmount,   // Используем актуальный баланс токенов
		ms.config.InitialAmount, // Сумма вложения SOL
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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
func (ms *MonitoringSession) updateBalanceAndCalculatePnL(ctx context.Context, currentAmount float64) (float64, *PnLData, error) {
	// Актуализация баланса
	updatedBalance := currentAmount
	tokenBalanceRaw, err := ms.config.DEX.GetTokenBalance(ctx, ms.config.TokenMint)
	if err == nil && tokenBalanceRaw > 0 {
		newBalance := float64(tokenBalanceRaw) / 1e6
		if math.Abs(newBalance-currentAmount) > 0.000001 && newBalance > 0 {
			ms.logger.Debug("Token balance changed",
				zap.Float64("old_balance", currentAmount),
				zap.Float64("new_balance", newBalance))

			ms.config.TokenAmount = newBalance
			ms.config.TokenBalance = tokenBalanceRaw
			updatedBalance = newBalance
		}
	}

	// Получение калькулятора и расчет PnL
	calculator, err := GetCalculator(ms.config.DEX, ms.logger)
	if err != nil {
		ms.logger.Error("Failed to get calculator for DEX", zap.Error(err))
		fmt.Printf("\nError: Cannot calculate PnL for %s\n", ms.config.DEX.GetName())
		return updatedBalance, nil, err
	}

	pnlData, err := calculator.CalculatePnL(ctx, ms.config.TokenMint, updatedBalance, ms.config.InitialAmount)
	if err != nil {
		ms.logger.Error("Failed to calculate PnL", zap.Error(err))
		fmt.Printf("\nError calculating PnL: %v\n", err)
		return updatedBalance, nil, err
	}

	return updatedBalance, pnlData, nil
}

// displayMonitorInfo форматирует и выводит информацию о мониторинге в консоль.
func (ms *MonitoringSession) displayMonitorInfo(currentPrice, initialPrice, percentChange, tokenBalance float64, pnlData *PnLData) {
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
	fmt.Printf("║ Token: %-38s ║\n", shortenAddress(ms.config.TokenMint))
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
	fmt.Println("\nSelling tokens...")

	// Останавливаем сессию мониторинга ДО выполнения операции продажи
	// Это предотвратит дальнейшие попытки обновления цены
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
