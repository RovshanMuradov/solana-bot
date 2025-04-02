package monitor

import (
	"context"
	"fmt"
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

	// Clear screen and show initial message
	fmt.Print("\033[H\033[2J") // Clear screen
	fmt.Println("\n🚀 Monitoring started. Press Enter to sell tokens or 'q' to exit.")
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

// onPriceUpdate is called when the price is updated
func (ms *MonitoringSession) onPriceUpdate(currentPrice, initialPrice, percentChange, tokenAmount float64) {
	// Clear screen for each update
	fmt.Print("\033[H\033[2J")
	
	// Стандартный расчет PnL
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
	fmt.Println("\n")
	fmt.Println("┌─────────────────────────────────────────────────────────────────┐")
	fmt.Printf("│ \033[1;36m%-63s\033[0m │\n", "SOLANA TRADING BOT - PRICE MONITOR")
	fmt.Println("├─────────────────────────────────────────────────────────────────┤")
	fmt.Printf("│ \033[1;33m%-20s\033[0m %-41s │\n", "Exchange:", ms.config.DEX.GetName())
	fmt.Printf("│ \033[1;33m%-20s\033[0m %-41s │\n", "Token:", shortenAddress(ms.config.TokenMint))
	fmt.Println("├─────────────────────────────────────────────────────────────────┤")
	
	if err == nil && discretePnL != nil {
		// Используем дискретный расчет
		fmt.Printf("│ \033[1;37m%-20s\033[0m %.9f SOL                    │\n", "Entry Price:", initialPrice)
		fmt.Printf("│ \033[1;37m%-20s\033[0m %.9f SOL                    │\n", "Current Price:", discretePnL.CurrentPrice)
		
		// Price change with color
		fmt.Printf("│ \033[1;37m%-20s\033[0m ", "Price Change:")
		printColoredText("%.2f%%%-39s │\n", percentChange, percentChange > 0, "")
		
		fmt.Printf("│ \033[1;37m%-20s\033[0m %.6f                          │\n", "Token Amount:", tokenAmount)
		fmt.Println("├─────────────────────────────────────────────────────────────────┤")
		fmt.Printf("│ \033[1;37m%-20s\033[0m %.6f SOL                      │\n", "Initial Investment:", discretePnL.InitialInvestment)
		fmt.Printf("│ \033[1;37m%-20s\033[0m %.6f SOL                      │\n", "Theoretical Value:", discretePnL.TheoreticalValue)
		fmt.Printf("│ \033[1;37m%-20s\033[0m %.6f SOL                      │\n", "Sell Estimate:", discretePnL.SellEstimate)
		
		// Print Net PnL with color
		fmt.Printf("│ \033[1;37m%-20s\033[0m ", "Net PnL:")
		printColoredText("%.6f SOL%-29s │\n", discretePnL.NetPnL, discretePnL.NetPnL > 0, "")
		
		// Print PnL percentage with color
		fmt.Printf("│ \033[1;37m%-20s\033[0m ", "ROI:")
		printColoredText("%.2f%%%-39s │\n", discretePnL.PnLPercentage, discretePnL.PnLPercentage > 0, "")
	} else {
		// Используем стандартный расчет
		fmt.Printf("│ \033[1;37m%-20s\033[0m %.9f SOL                    │\n", "Entry Price:", initialPrice)
		fmt.Printf("│ \033[1;37m%-20s\033[0m %.9f SOL                    │\n", "Current Price:", currentPrice)
		
		// Price change with color
		fmt.Printf("│ \033[1;37m%-20s\033[0m ", "Price Change:")
		printColoredText("%.2f%%%-39s │\n", percentChange, percentChange > 0, "")
		
		fmt.Printf("│ \033[1;37m%-20s\033[0m %.6f                          │\n", "Token Amount:", tokenAmount)
		fmt.Println("├─────────────────────────────────────────────────────────────────┤")
		fmt.Printf("│ \033[1;37m%-20s\033[0m %.6f SOL                      │\n", "Initial Investment:", ms.config.InitialAmount)
		fmt.Printf("│ \033[1;37m%-20s\033[0m %.6f SOL                      │\n", "Current Value:", currentValue)
		
		// Print Net PnL with color
		fmt.Printf("│ \033[1;37m%-20s\033[0m ", "Net PnL:")
		printColoredText("%.6f SOL%-29s │\n", profit, profit > 0, "")
		
		// Print PnL percentage with color
		fmt.Printf("│ \033[1;37m%-20s\033[0m ", "ROI:")
		printColoredText("%.2f%%%-39s │\n", profitPercent, profitPercent > 0, "")
	}
	
	fmt.Println("├─────────────────────────────────────────────────────────────────┤")
	fmt.Printf("│ \033[1;32m%-63s\033[0m │\n", "Press Enter to sell tokens or 'q' to exit")
	fmt.Println("└─────────────────────────────────────────────────────────────────┘")
	
	// Add update timestamp at the bottom
	fmt.Printf("\n\033[90mLast update: %s\033[0m\n", time.Now().Format("15:04:05"))
}

// shortenAddress сокращает адрес, показывая только начало и конец
func shortenAddress(address string) string {
	if len(address) > 12 {
		return address[:6] + "..." + address[len(address)-4:]
	}
	return address
}

// onEnterPressed is called when Enter is pressed
func (ms *MonitoringSession) onEnterPressed(_ string) error {
	// Clear screen
	fmt.Print("\033[H\033[2J")
	fmt.Println("\n🚀 Selling tokens...")

	// Stop the monitoring session
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
		fmt.Printf("\n\033[31mError selling tokens: %v\033[0m\n", err)
		return err
	}

	fmt.Println("\n\033[32m✅ Tokens sold successfully!\033[0m")
	return nil
}

// onExitCommand is called when exit command is entered
func (ms *MonitoringSession) onExitCommand(_ string) error {
	// Clear screen
	fmt.Print("\033[H\033[2J")
	fmt.Println("\n\033[33mExiting monitor mode without selling tokens.\033[0m")
	ms.Stop()
	return nil
}