package monitor

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestTradeHistoryConcurrentAccess(t *testing.T) {
	logger := zap.NewNop()
	tempDir := t.TempDir()

	history, err := NewTradeHistory(tempDir, 100, logger)
	if err != nil {
		t.Fatalf("Failed to create trade history: %v", err)
	}
	defer history.Close()

	var wg sync.WaitGroup
	numGoroutines := 10
	tradesPerGoroutine := 50

	// Concurrent trade logging
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < tradesPerGoroutine; j++ {
				trade := Trade{
					Timestamp:   time.Now(),
					WalletAddr:  fmt.Sprintf("wallet_%d", id),
					TokenMint:   fmt.Sprintf("token_%d_%d", id, j),
					TokenSymbol: fmt.Sprintf("TKN%d", j),
					Action:      "buy",
					AmountSOL:   float64(j) * 0.1,
					AmountToken: float64(j) * 100,
					Price:       0.001,
					TxSignature: fmt.Sprintf("sig_%d_%d", id, j),
					Success:     true,
				}

				if j%2 == 0 {
					trade.Action = "sell"
					trade.PnL = float64(j) * 0.01
					trade.PnLPercent = float64(j)
				}

				if err := history.LogTrade(trade); err != nil {
					t.Errorf("Failed to log trade: %v", err)
				}
			}
		}(i)
	}

	// Concurrent reads
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				trades := history.GetRecentTrades(10)
				_ = trades

				tokenMint := fmt.Sprintf("token_%d_%d", id, j)
				trades = history.GetTradesByToken(tokenMint)
				_ = trades

				stats := history.GetStatistics()
				_ = stats

				time.Sleep(5 * time.Millisecond)
			}
		}(i)
	}

	wg.Wait()

	// Verify results
	stats := history.GetStatistics()
	expectedTrades := numGoroutines * tradesPerGoroutine

	if stats.TotalTrades != expectedTrades {
		t.Errorf("Expected %d total trades, got %d", expectedTrades, stats.TotalTrades)
	}

	if stats.SuccessRate != 100 {
		t.Errorf("Expected 100%% success rate, got %.1f%%", stats.SuccessRate)
	}

	t.Logf("Trade history stats: %+v", stats)
}

func TestTradeHistoryCSVWriting(t *testing.T) {
	logger := zap.NewNop()
	tempDir := t.TempDir()

	history, err := NewTradeHistory(tempDir, 10, logger)
	if err != nil {
		t.Fatalf("Failed to create trade history: %v", err)
	}
	defer history.Close()

	// Log some trades
	trades := []Trade{
		{
			ID:          "trade1",
			Timestamp:   time.Now(),
			WalletAddr:  "wallet1",
			TokenMint:   "token1",
			TokenSymbol: "TKN1",
			Action:      "buy",
			AmountSOL:   1.0,
			AmountToken: 1000,
			Price:       0.001,
			TxSignature: "sig1",
			Success:     true,
		},
		{
			ID:          "trade2",
			Timestamp:   time.Now().Add(1 * time.Minute),
			WalletAddr:  "wallet1",
			TokenMint:   "token1",
			TokenSymbol: "TKN1",
			Action:      "sell",
			AmountSOL:   1.5,
			AmountToken: 1000,
			Price:       0.0015,
			EntryPrice:  0.001,
			ExitPrice:   0.0015,
			PnL:         0.5,
			PnLPercent:  50,
			HoldTime:    "1m",
			TxSignature: "sig2",
			Success:     true,
		},
	}

	for _, trade := range trades {
		if err := history.LogTrade(trade); err != nil {
			t.Fatalf("Failed to log trade: %v", err)
		}
	}

	// Force flush
	if err := history.Flush(); err != nil {
		t.Errorf("Failed to flush: %v", err)
	}

	// Check recent trades
	recent := history.GetRecentTrades(10)
	if len(recent) != 2 {
		t.Errorf("Expected 2 recent trades, got %d", len(recent))
	}

	// Check trade by ID
	trade, exists := history.GetTradeByID("trade1")
	if !exists {
		t.Error("Trade1 should exist")
	}
	if trade != nil && trade.Action != "buy" {
		t.Errorf("Expected buy action, got %s", trade.Action)
	}

	// Check trades by token
	tokenTrades := history.GetTradesByToken("token1")
	if len(tokenTrades) != 2 {
		t.Errorf("Expected 2 trades for token1, got %d", len(tokenTrades))
	}

	// Check statistics
	stats := history.GetStatistics()
	if stats.TotalTrades != 2 {
		t.Errorf("Expected 2 total trades, got %d", stats.TotalTrades)
	}
	if stats.TotalPnL != 0.5 {
		t.Errorf("Expected 0.5 total PnL, got %.4f", stats.TotalPnL)
	}
	if stats.WinRate != 100 {
		t.Errorf("Expected 100%% win rate, got %.1f%%", stats.WinRate)
	}
}

func TestTradeHistoryCircularBuffer(t *testing.T) {
	logger := zap.NewNop()
	tempDir := t.TempDir()

	// Small buffer size for testing
	maxTrades := 5
	history, err := NewTradeHistory(tempDir, maxTrades, logger)
	if err != nil {
		t.Fatalf("Failed to create trade history: %v", err)
	}
	defer history.Close()

	// Log more trades than buffer size
	for i := 0; i < 10; i++ {
		trade := Trade{
			ID:          fmt.Sprintf("trade%d", i),
			Timestamp:   time.Now(),
			TokenMint:   fmt.Sprintf("token%d", i),
			TokenSymbol: fmt.Sprintf("TKN%d", i),
			Action:      "buy",
			AmountSOL:   1.0,
			Success:     true,
		}

		if err := history.LogTrade(trade); err != nil {
			t.Fatalf("Failed to log trade: %v", err)
		}
	}

	// Check that only most recent trades are kept
	recent := history.GetRecentTrades(10)
	if len(recent) != maxTrades {
		t.Errorf("Expected %d trades in memory, got %d", maxTrades, len(recent))
	}

	// Verify these are the most recent trades
	for i, trade := range recent {
		expectedID := fmt.Sprintf("trade%d", i+5) // trades 5-9
		if trade.ID != expectedID {
			t.Errorf("Expected trade ID %s, got %s", expectedID, trade.ID)
		}
	}

	// But total count should include all trades
	stats := history.GetStatistics()
	if stats.TotalTrades != 10 {
		t.Errorf("Expected 10 total trades logged, got %d", stats.TotalTrades)
	}
}
