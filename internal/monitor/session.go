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

// MonitoringSession manages a token monitoring session
type MonitoringSession struct {
	config       *SessionConfig
	priceMonitor *PriceMonitor
	inputHandler *InputHandler
	wg           sync.WaitGroup
	ctx          context.Context
	cancel       context.CancelFunc
	logger       *zap.Logger
}

// NewMonitoringSession creates a new monitoring session
func NewMonitoringSession(config *SessionConfig) *MonitoringSession {
	ctx, cancel := context.WithCancel(context.Background())
	return &MonitoringSession{
		config: config,
		logger: config.Logger,
		ctx:    ctx,
		cancel: cancel,
	}
}

// Start begins the monitoring session
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

// Wait waits for the session to complete
func (ms *MonitoringSession) Wait() error {
	ms.wg.Wait()
	return nil
}

// Stop stops the monitoring session
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

// UpdateWithDiscretePnL обновляет сессию мониторинга с учетом дискретного расчета PnL
func (ms *MonitoringSession) UpdateWithDiscretePnL() error {
	// Меняем callback для обработки обновлений цены
	ms.priceMonitor.SetCallback(func(currentPrice, initialPrice, percentChange, tokenAmount float64) {
		// Используем измененную функцию onPriceUpdate
		ms.onPriceUpdate(currentPrice, initialPrice, percentChange, tokenAmount)
	})

	return nil
}

// printColoredText выводит текст с цветом в зависимости от значения
func printColoredText(format string, value float64, isPositive bool, args ...interface{}) {
	var colorCode string

	if value == 0 {
		colorCode = "\033[0m" // Default color
	} else if isPositive {
		colorCode = "\033[32m" // Green
	} else {
		colorCode = "\033[31m" // Red
	}

	allArgs := append([]interface{}{value}, args...)
	fmt.Printf(colorCode+format+"\033[0m", allArgs...)
}

// internal/monitor/session.go (фрагмент - функция onPriceUpdate)

// onPriceUpdate вызывается при обновлении цены
func (ms *MonitoringSession) onPriceUpdate(currentPrice, initialPrice, percentChange, _ float64) {
	// Получаем актуальный баланс токенов через RPC
	balanceCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	actualTokenBalance, err := ms.config.DEX.GetTokenBalance(balanceCtx, ms.config.TokenMint)

	// Рассчитываем актуальное человекочитаемое количество токенов
	tokenAmount := ms.config.TokenAmount // Используем старое значение, если не удалось получить новое
	if err == nil && actualTokenBalance > 0 {
		// Конвертируем актуальный баланс в человекочитаемый формат
		tokenDecimals := 6 // По умолчанию 6 знаков (стандарт для многих токенов)
		tokenAmount = float64(actualTokenBalance) / math.Pow10(int(tokenDecimals))

		ms.logger.Debug("Updated token balance",
			zap.Uint64("raw_balance", actualTokenBalance),
			zap.Float64("human_amount", tokenAmount))
	} else if err != nil {
		ms.logger.Debug("Could not fetch actual token balance", zap.Error(err))
	}

	// Стандартный расчет PnL с актуальным балансом
	currentValue := currentPrice * tokenAmount
	profit := currentValue - ms.config.InitialAmount
	profitPercent := 0.0
	if ms.config.InitialAmount > 0 {
		profitPercent = (profit / ms.config.InitialAmount) * 100
	}

	// Пытаемся вычислить более точный PnL для Pump.fun
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	discretePnL, err := ms.config.DEX.CalculateDiscretePnL(ctx, tokenAmount, ms.config.InitialAmount)

	// Компактный однострочный формат вывода
	var pnlText string

	if err == nil && discretePnL != nil {
		// Цветовое оформление для изменения цены и PnL
		priceChangeColor := "\033[0m" // Нейтральный
		if percentChange > 0 {
			priceChangeColor = "\033[32m" // Зеленый
		} else if percentChange < 0 {
			priceChangeColor = "\033[31m" // Красный
		}

		pnlColor := "\033[0m" // Нейтральный
		if discretePnL.NetPnL > 0 {
			pnlColor = "\033[32m" // Зеленый
		} else if discretePnL.NetPnL < 0 {
			pnlColor = "\033[31m" // Красный
		}

		// Форматирование в одну строку
		pnlText = fmt.Sprintf("\n=== %s Discrete PnL ===\n", ms.config.DEX.GetName()) +
			fmt.Sprintf("Entry Price: %.9f SOL | Current Price: %.9f SOL | Change: %s%.2f%%\033[0m\n",
				initialPrice, discretePnL.CurrentPrice, priceChangeColor, percentChange) +
			fmt.Sprintf("Tokens: %.6f | Theoretical Value: %.6f SOL | Sell Estimate: %.6f SOL\n",
				tokenAmount, discretePnL.TheoreticalValue, discretePnL.SellEstimate) +
			fmt.Sprintf("Initial Investment: %.6f SOL | Net PnL: %s%.6f SOL (%.2f%%)\033[0m\n",
				discretePnL.InitialInvestment, pnlColor, discretePnL.NetPnL, discretePnL.PnLPercentage) +
			fmt.Sprintf("===========================\n")
	} else {
		// Стандартный расчет, если дискретный недоступен
		priceChangeColor := "\033[0m" // Нейтральный
		if percentChange > 0 {
			priceChangeColor = "\033[32m" // Зеленый
		} else if percentChange < 0 {
			priceChangeColor = "\033[31m" // Красный
		}

		pnlColor := "\033[0m" // Нейтральный
		if profit > 0 {
			pnlColor = "\033[32m" // Зеленый
		} else if profit < 0 {
			pnlColor = "\033[31m" // Красный
		}

		pnlText = fmt.Sprintf("\n=== %s PnL ===\n", ms.config.DEX.GetName()) +
			fmt.Sprintf("Entry Price: %.9f SOL | Current Price: %.9f SOL | Change: %s%.2f%%\033[0m\n",
				initialPrice, currentPrice, priceChangeColor, percentChange) +
			fmt.Sprintf("Tokens: %.6f | Value: %.6f SOL\n", tokenAmount, currentValue) +
			fmt.Sprintf("Initial Investment: %.6f SOL | Net PnL: %s%.6f SOL (%.2f%%)\033[0m\n",
				ms.config.InitialAmount, pnlColor, profit, profitPercent) +
			fmt.Sprintf("===========================\n")
	}

	// Вывод информации и инструкции
	fmt.Println(pnlText)
	fmt.Println("Press Enter to sell tokens or 'q' to exit.")
}

// shortenAddress сокращает адрес, показывая только начало и конец
func shortenAddress(address string) string {
	if len(address) > 12 {
		return address[:6] + "..." + address[len(address)-4:]
	}
	return address
}

// onEnterPressed вызывается при нажатии Enter
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

// onExitCommand is called when exit command is entered
func (ms *MonitoringSession) onExitCommand(_ string) error {
	fmt.Println("\nExiting monitor mode without selling tokens.")
	ms.Stop()
	return nil
}
