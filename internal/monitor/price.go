package monitor

import (
	"context"
	"math"
	"time"

	"github.com/rovshanmuradov/solana-bot/internal/dex"
	"go.uber.org/zap"
)

// PriceUpdateCallback is a function called when price updates
type PriceUpdateCallback func(currentPriceSol float64, initialPriceSol float64, percentChange float64, tokenAmount float64)

// PriceMonitor monitors token price changes
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
}

// NewPriceMonitor creates a new price monitor
func NewPriceMonitor(dex dex.DEX, tokenMint string, initialPrice float64,
	tokenAmount float64, initialAmount float64,
	interval time.Duration, logger *zap.Logger,
	callback PriceUpdateCallback) *PriceMonitor {
	ctx, cancel := context.WithCancel(context.Background())
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

// Start begins the price monitoring process
func (pm *PriceMonitor) Start() {
	pm.logger.Info("Starting price monitor",
		zap.String("token_mint", pm.tokenMint),
		zap.Float64("initial_price", pm.initialPrice),
		zap.Duration("interval", pm.interval))

	// Run the first update immediately
	pm.updatePrice()

	// Start the ticker for periodic updates
	ticker := time.NewTicker(pm.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			pm.updatePrice()
		case <-pm.ctx.Done():
			pm.logger.Debug("Price monitor stopped")
			return
		}
	}
}

// Stop stops the price monitoring
func (pm *PriceMonitor) Stop() {
	if pm.cancel != nil {
		pm.cancel()
	}
}

// updatePrice fetches the current price and calls the callback
func (pm *PriceMonitor) updatePrice() {
	ctx, cancel := context.WithTimeout(pm.ctx, 10*time.Second)
	defer cancel()

	currentPrice, err := pm.dex.GetTokenPrice(ctx, pm.tokenMint)
	if err != nil {
		pm.logger.Error("Failed to get token price", zap.Error(err))
		return
	}

	// Calculate percent change
	percentChange := 0.0
	if pm.initialPrice > 0 {
		percentChange = ((currentPrice - pm.initialPrice) / pm.initialPrice) * 100
	}

	// Format to 2 decimal places
	percentChange = math.Floor(percentChange*100) / 100

	// Call the callback with price information
	if pm.callback != nil {
		pm.callback(currentPrice, pm.initialPrice, percentChange, pm.tokenAmount)
	}
}
