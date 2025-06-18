package monitor

import (
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/rovshanmuradov/solana-bot/internal/logger"
	"go.uber.org/zap"
)

// TradeHistory manages trade logging and history
type TradeHistory struct {
	mu        sync.RWMutex
	csvWriter *logger.SafeCSVWriter
	trades    []Trade
	maxTrades int
	logger    *zap.Logger

	// Statistics
	totalTrades      int
	successfulTrades int
	totalVolume      float64
	totalPnL         float64
}

// NewTradeHistory creates a new trade history manager
func NewTradeHistory(logDir string, maxTrades int, zapLogger *zap.Logger) (*TradeHistory, error) {
	// Create trades directory
	tradesDir := filepath.Join(logDir, "trades")

	// Create CSV file with timestamp
	filename := fmt.Sprintf("trades_%s.csv", time.Now().Format("20060102_150405"))
	csvPath := filepath.Join(tradesDir, filename)

	// Create CSV writer with 30 second flush interval
	csvWriter, err := logger.NewSafeCSVWriter(csvPath, 30*time.Second, zapLogger)
	if err != nil {
		return nil, fmt.Errorf("failed to create CSV writer: %w", err)
	}

	// Write headers
	if err := csvWriter.WriteRecord(CSVHeaders()); err != nil {
		csvWriter.Close()
		return nil, fmt.Errorf("failed to write CSV headers: %w", err)
	}

	th := &TradeHistory{
		csvWriter: csvWriter,
		trades:    make([]Trade, 0, maxTrades),
		maxTrades: maxTrades,
		logger:    zapLogger,
	}

	zapLogger.Info("Trade history initialized",
		zap.String("csv_file", csvPath),
		zap.Int("max_memory_trades", maxTrades))

	return th, nil
}

// LogTrade logs a new trade
func (th *TradeHistory) LogTrade(trade Trade) error {
	th.mu.Lock()
	defer th.mu.Unlock()

	// Generate ID if not set
	if trade.ID == "" {
		trade.ID = fmt.Sprintf("%s_%d", trade.TokenMint[:8], time.Now().UnixNano())
	}

	// Ensure timestamp
	if trade.Timestamp.IsZero() {
		trade.Timestamp = time.Now()
	}

	// Write to CSV
	if err := th.csvWriter.WriteRecord(trade.ToCSV()); err != nil {
		th.logger.Error("Failed to write trade to CSV",
			zap.String("trade_id", trade.ID),
			zap.Error(err))
		return fmt.Errorf("failed to write trade: %w", err)
	}

	// Add to memory (circular buffer)
	if len(th.trades) >= th.maxTrades {
		// Remove oldest trade
		th.trades = th.trades[1:]
	}
	th.trades = append(th.trades, trade)

	// Update statistics
	th.totalTrades++
	if trade.Success {
		th.successfulTrades++
	}
	if trade.Action == "buy" || trade.Action == "sell" {
		th.totalVolume += trade.AmountSOL
	}
	if trade.PnL != 0 {
		th.totalPnL += trade.PnL
	}

	th.logger.Info("Trade logged",
		zap.String("id", trade.ID),
		zap.String("action", trade.Action),
		zap.String("token", trade.TokenSymbol),
		zap.Float64("amount_sol", trade.AmountSOL),
		zap.Bool("success", trade.Success))

	return nil
}

// GetRecentTrades returns recent trades from memory
func (th *TradeHistory) GetRecentTrades(limit int) []Trade {
	th.mu.RLock()
	defer th.mu.RUnlock()

	if limit <= 0 || limit > len(th.trades) {
		limit = len(th.trades)
	}

	// Return most recent trades
	start := len(th.trades) - limit
	if start < 0 {
		start = 0
	}

	result := make([]Trade, limit)
	copy(result, th.trades[start:])

	return result
}

// GetTradeByID returns a specific trade by ID
func (th *TradeHistory) GetTradeByID(id string) (*Trade, bool) {
	th.mu.RLock()
	defer th.mu.RUnlock()

	for i := len(th.trades) - 1; i >= 0; i-- {
		if th.trades[i].ID == id {
			trade := th.trades[i]
			return &trade, true
		}
	}

	return nil, false
}

// GetTradesByToken returns all trades for a specific token
func (th *TradeHistory) GetTradesByToken(tokenMint string) []Trade {
	th.mu.RLock()
	defer th.mu.RUnlock()

	var result []Trade
	for _, trade := range th.trades {
		if trade.TokenMint == tokenMint {
			result = append(result, trade)
		}
	}

	return result
}

// GetStatistics returns trading statistics
func (th *TradeHistory) GetStatistics() TradeStatistics {
	th.mu.RLock()
	defer th.mu.RUnlock()

	stats := TradeStatistics{
		TotalTrades:      th.totalTrades,
		SuccessfulTrades: th.successfulTrades,
		FailedTrades:     th.totalTrades - th.successfulTrades,
		TotalVolume:      th.totalVolume,
		TotalPnL:         th.totalPnL,
	}

	if th.totalTrades > 0 {
		stats.SuccessRate = float64(th.successfulTrades) / float64(th.totalTrades) * 100
	}

	// Calculate additional metrics from recent trades
	var (
		winCount     int
		totalWinPnL  float64
		totalLossPnL float64
		buyCount     int
		sellCount    int
	)

	for _, trade := range th.trades {
		if trade.Action == "buy" {
			buyCount++
		} else if trade.Action == "sell" {
			sellCount++
			if trade.PnL > 0 {
				winCount++
				totalWinPnL += trade.PnL
			} else if trade.PnL < 0 {
				totalLossPnL += trade.PnL
			}
		}
	}

	stats.BuyCount = buyCount
	stats.SellCount = sellCount

	if sellCount > 0 {
		stats.WinRate = float64(winCount) / float64(sellCount) * 100
	}

	if winCount > 0 {
		stats.AvgWinPnL = totalWinPnL / float64(winCount)
	}

	lossCount := sellCount - winCount
	if lossCount > 0 {
		stats.AvgLossPnL = totalLossPnL / float64(lossCount)
	}

	return stats
}

// Flush forces a write of any buffered trades
func (th *TradeHistory) Flush() error {
	return th.csvWriter.Flush()
}

// Close closes the trade history and ensures all data is written
func (th *TradeHistory) Close() error {
	th.mu.Lock()
	defer th.mu.Unlock()

	stats := th.GetStatistics()
	th.logger.Info("Closing trade history",
		zap.Int("total_trades", stats.TotalTrades),
		zap.Float64("total_volume", stats.TotalVolume),
		zap.Float64("total_pnl", stats.TotalPnL),
		zap.Float64("success_rate", stats.SuccessRate))

	return th.csvWriter.Close()
}

// TradeStatistics holds aggregate trade statistics
type TradeStatistics struct {
	TotalTrades      int     `json:"total_trades"`
	SuccessfulTrades int     `json:"successful_trades"`
	FailedTrades     int     `json:"failed_trades"`
	SuccessRate      float64 `json:"success_rate"`
	BuyCount         int     `json:"buy_count"`
	SellCount        int     `json:"sell_count"`
	TotalVolume      float64 `json:"total_volume"`
	TotalPnL         float64 `json:"total_pnl"`
	WinRate          float64 `json:"win_rate"`
	AvgWinPnL        float64 `json:"avg_win_pnl"`
	AvgLossPnL       float64 `json:"avg_loss_pnl"`
}
