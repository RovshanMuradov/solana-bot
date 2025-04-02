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

// onPriceUpdate is called when the price is updated
func (ms *MonitoringSession) onPriceUpdate(currentPrice, initialPrice, percentChange, tokenAmount float64) {
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
	if err == nil && discretePnL != nil {
		// Используем дискретный расчет
		fmt.Printf("\n=== Pump.fun Discrete PnL ===\n")
		fmt.Printf("Entry Price: %.9f SOL | Current Price: %.9f SOL | Change: %.2f%%\n",
			initialPrice, discretePnL.CurrentPrice, percentChange)
		fmt.Printf("Tokens: %.6f | Theoretical Value: %.6f SOL | Sell Estimate: %.6f SOL\n",
			tokenAmount, discretePnL.TheoreticalValue, discretePnL.SellEstimate)
		fmt.Printf("Initial Investment: %.6f SOL | Net PnL: %.6f SOL (%.2f%%)\n",
			discretePnL.InitialInvestment, discretePnL.NetPnL, discretePnL.PnLPercentage)
		fmt.Printf("===========================\n")
	} else {
		// Используем стандартный расчет
		ms.logger.Debug("Using standard PnL calculation", zap.Error(err))
		fmt.Printf("\nEntry Price: %.9f SOL | Current Price: %.9f SOL | Change: %.2f%%\n",
			initialPrice, currentPrice, percentChange)
		fmt.Printf("Tokens: %.6f | Value: %.6f SOL | P&L: %.6f SOL (ROI: %.2f%%)\n",
			tokenAmount, currentValue, profit, profitPercent)
	}

	fmt.Println("\nPress Enter to sell tokens or 'q' to exit.")
}

// onEnterPressed is called when Enter is pressed
func (ms *MonitoringSession) onEnterPressed(_ string) error {
	fmt.Println("Selling tokens...")

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
		fmt.Printf("Error selling tokens: %v\n", err)
		return err
	}

	fmt.Println("Tokens sold successfully!")
	return nil
}

// onExitCommand is called when exit command is entered
func (ms *MonitoringSession) onExitCommand(_ string) error {
	fmt.Println("Exiting monitor mode without selling tokens.")
	ms.Stop()
	return nil
}
