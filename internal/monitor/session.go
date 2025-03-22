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
	TokenAmount     float64       // Amount of tokens purchased
	InitialAmount   float64       // Initial SOL amount spent
	InitialPrice    float64       // Initial token price
	MonitorInterval time.Duration // Interval for price updates
	DEX             dex.DEX       // DEX interface
	Logger          *zap.Logger   // Logger
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

// onPriceUpdate is called when the price is updated
func (ms *MonitoringSession) onPriceUpdate(currentPrice, _ /*initialPrice*/, percentChange, tokenAmount float64) {
	// Calculate current value and profit
	currentValue := currentPrice * tokenAmount
	profit := currentValue - ms.config.InitialAmount
	profitPercent := 0.0
	if ms.config.InitialAmount > 0 {
		profitPercent = (profit / ms.config.InitialAmount) * 100
	}

	// Display price information
	fmt.Printf("Price: %.9f SOL | Change: %.2f%% | Value: %.6f SOL | Profit: %.6f SOL (%.2f%%)\n",
		currentPrice, percentChange, currentValue, profit, profitPercent)
}

// onEnterPressed is called when Enter is pressed
func (ms *MonitoringSession) onEnterPressed(_ string) error {
	fmt.Println("Selling tokens...")

	// Stop the monitoring session
	ms.Stop()

	// Create the sell task
	sellTask := &dex.Task{
		Operation:       dex.OperationSell,
		TokenMint:       ms.config.TokenMint,
		AmountSol:       ms.config.TokenAmount, // AmountSol is used for token amount in sell operations
		SlippagePercent: 1.0,                   // Default slippage
		PriorityFee:     "0.000001",            // Default priority fee
		ComputeUnits:    300000,                // Default compute units
	}

	// Execute the sell operation
	err := ms.config.DEX.Execute(context.Background(), sellTask)
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
