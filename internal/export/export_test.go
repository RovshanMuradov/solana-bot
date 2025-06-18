package export

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/rovshanmuradov/solana-bot/internal/monitor"
	"go.uber.org/zap"
)

func TestTradeExportCSV(t *testing.T) {
	logger := zap.NewNop()
	exporter := NewTradeExporter(logger)
	tempDir := t.TempDir()

	// Create test trades
	trades := generateTestTrades()

	// Export to CSV
	options := ExportOptions{
		Format:    FormatCSV,
		OutputDir: tempDir,
	}

	outputPath, err := exporter.ExportTrades(trades, options)
	if err != nil {
		t.Fatalf("Failed to export trades: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Error("Export file does not exist")
	}

	// Check file size
	info, err := os.Stat(outputPath)
	if err != nil {
		t.Fatalf("Failed to stat file: %v", err)
	}
	if info.Size() == 0 {
		t.Error("Export file is empty")
	}

	t.Logf("Exported CSV to: %s (size: %d bytes)", outputPath, info.Size())
}

func TestTradeExportJSON(t *testing.T) {
	logger := zap.NewNop()
	exporter := NewTradeExporter(logger)
	tempDir := t.TempDir()

	// Create test trades
	trades := generateTestTrades()

	// Export to JSON
	options := ExportOptions{
		Format:    FormatJSON,
		OutputDir: tempDir,
	}

	outputPath, err := exporter.ExportTrades(trades, options)
	if err != nil {
		t.Fatalf("Failed to export trades: %v", err)
	}

	// Verify file exists and has content
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read export file: %v", err)
	}

	if len(content) == 0 {
		t.Error("Export file is empty")
	}

	t.Logf("Exported JSON to: %s (size: %d bytes)", outputPath, len(content))
}

func TestTradeExportFilters(t *testing.T) {
	logger := zap.NewNop()
	exporter := NewTradeExporter(logger)
	tempDir := t.TempDir()

	// Create test trades
	trades := generateTestTrades()

	// Test time filter
	options := ExportOptions{
		Format:    FormatCSV,
		StartTime: time.Now().Add(-30 * time.Minute),
		EndTime:   time.Now().Add(-10 * time.Minute),
		OutputDir: tempDir,
	}

	outputPath, err := exporter.ExportTrades(trades, options)
	if err != nil {
		t.Fatalf("Failed to export with time filter: %v", err)
	}
	t.Logf("Time filtered export: %s", outputPath)

	// Test token filter
	options = ExportOptions{
		Format:      FormatCSV,
		TokenFilter: "token1",
		OutputDir:   tempDir,
	}

	outputPath, err = exporter.ExportTrades(trades, options)
	if err != nil {
		t.Fatalf("Failed to export with token filter: %v", err)
	}
	t.Logf("Token filtered export: %s", outputPath)

	// Test action filter
	options = ExportOptions{
		Format:       FormatCSV,
		ActionFilter: "sell",
		OutputDir:    tempDir,
	}

	outputPath, err = exporter.ExportTrades(trades, options)
	if err != nil {
		t.Fatalf("Failed to export with action filter: %v", err)
	}
	t.Logf("Action filtered export: %s", outputPath)

	// Test success filter
	options = ExportOptions{
		Format:      FormatCSV,
		OnlySuccess: true,
		OutputDir:   tempDir,
	}

	outputPath, err = exporter.ExportTrades(trades, options)
	if err != nil {
		t.Fatalf("Failed to export with success filter: %v", err)
	}
	t.Logf("Success filtered export: %s", outputPath)
}

func TestDailyReportExport(t *testing.T) {
	logger := zap.NewNop()
	exporter := NewTradeExporter(logger)
	tempDir := t.TempDir()

	// Create test trades
	trades := generateTestTrades()

	// Export daily report
	outputPath, err := exporter.ExportDailyReport(trades, time.Now(), tempDir)
	if err != nil {
		t.Fatalf("Failed to export daily report: %v", err)
	}

	if outputPath == "" {
		t.Log("No trades for today, which is expected in test")
		return
	}

	// Verify file exists
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Error("Daily report file does not exist")
	}

	t.Logf("Daily report exported to: %s", outputPath)
}

func TestExportSummaryCalculation(t *testing.T) {
	logger := zap.NewNop()
	exporter := NewTradeExporter(logger)

	trades := []monitor.Trade{
		{
			Timestamp: time.Now().Add(-2 * time.Hour),
			Action:    "buy",
			AmountSOL: 5.0,
			Success:   true,
		},
		{
			Timestamp: time.Now().Add(-1 * time.Hour),
			Action:    "sell",
			AmountSOL: 5.0,
			PnL:       2.5,
			Success:   true,
		},
		{
			Timestamp: time.Now().Add(-30 * time.Minute),
			Action:    "buy",
			AmountSOL: 3.0,
			Success:   true,
		},
		{
			Timestamp: time.Now(),
			Action:    "sell",
			AmountSOL: 3.0,
			PnL:       -0.5,
			Success:   true,
		},
	}

	summary := exporter.calculateSummary(trades)

	if summary.TotalTrades != 4 {
		t.Errorf("Expected 4 total trades, got %d", summary.TotalTrades)
	}

	if summary.BuyCount != 2 || summary.SellCount != 2 {
		t.Errorf("Expected 2 buys and 2 sells, got %d buys and %d sells",
			summary.BuyCount, summary.SellCount)
	}

	if summary.TotalVolume != 16.0 {
		t.Errorf("Expected total volume 16.0, got %.2f", summary.TotalVolume)
	}

	if summary.TotalPnL != 2.0 {
		t.Errorf("Expected total PnL 2.0, got %.2f", summary.TotalPnL)
	}

	if summary.WinRate != 50.0 {
		t.Errorf("Expected 50%% win rate, got %.1f%%", summary.WinRate)
	}

	t.Logf("Export summary: %+v", summary)
}

// Helper function to generate test trades
func generateTestTrades() []monitor.Trade {
	now := time.Now()
	trades := []monitor.Trade{
		{
			ID:          "trade1",
			Timestamp:   now.Add(-1 * time.Hour),
			TokenMint:   "token1",
			TokenSymbol: "TKN1",
			Action:      "buy",
			AmountSOL:   1.0,
			Success:     true,
		},
		{
			ID:          "trade2",
			Timestamp:   now.Add(-45 * time.Minute),
			TokenMint:   "token1",
			TokenSymbol: "TKN1",
			Action:      "sell",
			AmountSOL:   1.5,
			PnL:         0.5,
			Success:     true,
		},
		{
			ID:          "trade3",
			Timestamp:   now.Add(-30 * time.Minute),
			TokenMint:   "token2",
			TokenSymbol: "TKN2",
			Action:      "buy",
			AmountSOL:   2.0,
			Success:     true,
		},
		{
			ID:          "trade4",
			Timestamp:   now.Add(-20 * time.Minute),
			TokenMint:   "token2",
			TokenSymbol: "TKN2",
			Action:      "sell",
			AmountSOL:   1.8,
			PnL:         -0.2,
			Success:     false,
		},
		{
			ID:          "trade5",
			Timestamp:   now.Add(-10 * time.Minute),
			TokenMint:   "token3",
			TokenSymbol: "TKN3",
			Action:      "buy",
			AmountSOL:   5.0,
			Success:     true,
		},
	}

	return trades
}

func TestFilenameGeneration(t *testing.T) {
	logger := zap.NewNop()
	exporter := NewTradeExporter(logger)

	tests := []struct {
		options  ExportOptions
		expected string
	}{
		{
			options: ExportOptions{
				Format: FormatCSV,
			},
			expected: "trades_all",
		},
		{
			options: ExportOptions{
				Format:       FormatJSON,
				ActionFilter: "buy",
			},
			expected: "trades_buy",
		},
		{
			options: ExportOptions{
				Format:       FormatCSV,
				ActionFilter: "sell",
				TokenFilter:  "tokenABCD1234",
			},
			expected: "trades_sell_tokenABC",
		},
	}

	for _, tt := range tests {
		filename := exporter.generateFilename(tt.options)
		if !strings.HasPrefix(filename, tt.expected) {
			t.Errorf("Expected filename to start with %s, got %s", tt.expected, filename)
		}

		expectedExt := "." + string(tt.options.Format)
		if !strings.HasSuffix(filename, expectedExt) {
			t.Errorf("Expected filename to end with %s, got %s", expectedExt, filename)
		}
	}
}
