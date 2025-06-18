package export

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/rovshanmuradov/solana-bot/internal/monitor"
	"go.uber.org/zap"
)

// ExportFormat represents the export file format
type ExportFormat string

const (
	FormatCSV  ExportFormat = "csv"
	FormatJSON ExportFormat = "json"
)

// ExportOptions configures the export behavior
type ExportOptions struct {
	Format       ExportFormat
	StartTime    time.Time
	EndTime      time.Time
	TokenFilter  string // Filter by token mint
	ActionFilter string // Filter by action (buy/sell)
	OnlySuccess  bool   // Only export successful trades
	OutputDir    string
}

// TradeExporter handles trade export functionality
type TradeExporter struct {
	logger *zap.Logger
}

// NewTradeExporter creates a new trade exporter
func NewTradeExporter(logger *zap.Logger) *TradeExporter {
	return &TradeExporter{
		logger: logger,
	}
}

// ExportTrades exports trades based on the provided options
func (te *TradeExporter) ExportTrades(trades []monitor.Trade, options ExportOptions) (string, error) {
	// Filter trades
	filtered := te.filterTrades(trades, options)

	if len(filtered) == 0 {
		return "", fmt.Errorf("no trades match the export criteria")
	}

	// Sort by timestamp
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Timestamp.Before(filtered[j].Timestamp)
	})

	// Generate filename
	filename := te.generateFilename(options)
	outputPath := filepath.Join(options.OutputDir, filename)

	// Ensure output directory exists
	if err := os.MkdirAll(options.OutputDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create output directory: %w", err)
	}

	// Export based on format
	var err error
	switch options.Format {
	case FormatCSV:
		err = te.exportToCSV(filtered, outputPath)
	case FormatJSON:
		err = te.exportToJSON(filtered, outputPath)
	default:
		err = fmt.Errorf("unsupported format: %s", options.Format)
	}

	if err != nil {
		return "", err
	}

	te.logger.Info("Trades exported",
		zap.String("file", outputPath),
		zap.Int("count", len(filtered)),
		zap.String("format", string(options.Format)))

	return outputPath, nil
}

// filterTrades applies filters to the trade list
func (te *TradeExporter) filterTrades(trades []monitor.Trade, options ExportOptions) []monitor.Trade {
	var filtered []monitor.Trade

	for _, trade := range trades {
		// Time filter
		if !options.StartTime.IsZero() && trade.Timestamp.Before(options.StartTime) {
			continue
		}
		if !options.EndTime.IsZero() && trade.Timestamp.After(options.EndTime) {
			continue
		}

		// Token filter
		if options.TokenFilter != "" && trade.TokenMint != options.TokenFilter {
			continue
		}

		// Action filter
		if options.ActionFilter != "" && trade.Action != options.ActionFilter {
			continue
		}

		// Success filter
		if options.OnlySuccess && !trade.Success {
			continue
		}

		filtered = append(filtered, trade)
	}

	return filtered
}

// generateFilename creates a filename based on export options
func (te *TradeExporter) generateFilename(options ExportOptions) string {
	timestamp := time.Now().Format("20060102_150405")

	var prefix string
	if options.ActionFilter != "" {
		prefix = fmt.Sprintf("trades_%s", options.ActionFilter)
	} else {
		prefix = "trades_all"
	}

	if options.TokenFilter != "" {
		prefix += "_" + options.TokenFilter[:8]
	}

	return fmt.Sprintf("%s_%s.%s", prefix, timestamp, options.Format)
}

// exportToCSV exports trades to CSV format
func (te *TradeExporter) exportToCSV(trades []monitor.Trade, outputPath string) error {
	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create CSV file: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write headers
	if err := writer.Write(monitor.CSVHeaders()); err != nil {
		return fmt.Errorf("failed to write CSV headers: %w", err)
	}

	// Write trades
	for _, trade := range trades {
		if err := writer.Write(trade.ToCSV()); err != nil {
			return fmt.Errorf("failed to write trade: %w", err)
		}
	}

	return nil
}

// exportToJSON exports trades to JSON format
func (te *TradeExporter) exportToJSON(trades []monitor.Trade, outputPath string) error {
	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create JSON file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")

	// Create export data with metadata
	exportData := struct {
		ExportTime time.Time       `json:"export_time"`
		TradeCount int             `json:"trade_count"`
		Trades     []monitor.Trade `json:"trades"`
		Summary    ExportSummary   `json:"summary"`
	}{
		ExportTime: time.Now(),
		TradeCount: len(trades),
		Trades:     trades,
		Summary:    te.calculateSummary(trades),
	}

	if err := encoder.Encode(exportData); err != nil {
		return fmt.Errorf("failed to encode JSON: %w", err)
	}

	return nil
}

// calculateSummary calculates summary statistics for the export
func (te *TradeExporter) calculateSummary(trades []monitor.Trade) ExportSummary {
	summary := ExportSummary{
		TotalTrades: len(trades),
	}

	if len(trades) == 0 {
		return summary
	}

	// Calculate date range
	summary.StartDate = trades[0].Timestamp
	summary.EndDate = trades[len(trades)-1].Timestamp

	// Calculate statistics
	tokenSet := make(map[string]bool)

	for _, trade := range trades {
		tokenSet[trade.TokenMint] = true

		if trade.Success {
			summary.SuccessfulTrades++
		}

		if trade.Action == "buy" {
			summary.BuyCount++
			summary.TotalBuyVolume += trade.AmountSOL
		} else if trade.Action == "sell" {
			summary.SellCount++
			summary.TotalSellVolume += trade.AmountSOL
			summary.TotalPnL += trade.PnL

			if trade.PnL > 0 {
				summary.WinCount++
			} else if trade.PnL < 0 {
				summary.LossCount++
			}
		}
	}

	summary.UniqueTokens = len(tokenSet)
	summary.TotalVolume = summary.TotalBuyVolume + summary.TotalSellVolume

	if summary.SellCount > 0 {
		summary.WinRate = float64(summary.WinCount) / float64(summary.SellCount) * 100
		summary.AvgPnL = summary.TotalPnL / float64(summary.SellCount)
	}

	return summary
}

// ExportSummary contains summary statistics for exported trades
type ExportSummary struct {
	TotalTrades      int       `json:"total_trades"`
	SuccessfulTrades int       `json:"successful_trades"`
	BuyCount         int       `json:"buy_count"`
	SellCount        int       `json:"sell_count"`
	UniqueTokens     int       `json:"unique_tokens"`
	TotalVolume      float64   `json:"total_volume"`
	TotalBuyVolume   float64   `json:"total_buy_volume"`
	TotalSellVolume  float64   `json:"total_sell_volume"`
	TotalPnL         float64   `json:"total_pnl"`
	WinCount         int       `json:"win_count"`
	LossCount        int       `json:"loss_count"`
	WinRate          float64   `json:"win_rate"`
	AvgPnL           float64   `json:"avg_pnl"`
	StartDate        time.Time `json:"start_date"`
	EndDate          time.Time `json:"end_date"`
}

// ExportDailyReport exports a daily summary report
func (te *TradeExporter) ExportDailyReport(trades []monitor.Trade, date time.Time, outputDir string) (string, error) {
	// Filter trades for the specific day
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)

	options := ExportOptions{
		Format:    FormatJSON,
		StartTime: startOfDay,
		EndTime:   endOfDay,
		OutputDir: outputDir,
	}

	// Use a custom filename for daily reports
	filename := fmt.Sprintf("daily_report_%s.json", startOfDay.Format("20060102"))
	outputPath := filepath.Join(outputDir, filename)

	// Filter trades for the day
	filtered := te.filterTrades(trades, options)

	if len(filtered) == 0 {
		te.logger.Info("No trades for daily report",
			zap.Time("date", startOfDay))
		return "", nil
	}

	// Create daily report
	report := DailyReport{
		Date:            startOfDay,
		TradeCount:      len(filtered),
		Trades:          filtered,
		Summary:         te.calculateSummary(filtered),
		HourlyBreakdown: te.calculateHourlyBreakdown(filtered),
	}

	// Write report
	file, err := os.Create(outputPath)
	if err != nil {
		return "", fmt.Errorf("failed to create report file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(report); err != nil {
		return "", fmt.Errorf("failed to encode report: %w", err)
	}

	te.logger.Info("Daily report exported",
		zap.String("file", outputPath),
		zap.Time("date", startOfDay),
		zap.Int("trades", len(filtered)))

	return outputPath, nil
}

// DailyReport represents a daily trading report
type DailyReport struct {
	Date            time.Time       `json:"date"`
	TradeCount      int             `json:"trade_count"`
	Summary         ExportSummary   `json:"summary"`
	HourlyBreakdown []HourlyStats   `json:"hourly_breakdown"`
	Trades          []monitor.Trade `json:"trades"`
}

// HourlyStats represents trading statistics for an hour
type HourlyStats struct {
	Hour       int     `json:"hour"`
	TradeCount int     `json:"trade_count"`
	BuyCount   int     `json:"buy_count"`
	SellCount  int     `json:"sell_count"`
	Volume     float64 `json:"volume"`
	PnL        float64 `json:"pnl"`
}

// calculateHourlyBreakdown calculates hourly trading statistics
func (te *TradeExporter) calculateHourlyBreakdown(trades []monitor.Trade) []HourlyStats {
	hourlyMap := make(map[int]*HourlyStats)

	for _, trade := range trades {
		hour := trade.Timestamp.Hour()

		stats, exists := hourlyMap[hour]
		if !exists {
			stats = &HourlyStats{Hour: hour}
			hourlyMap[hour] = stats
		}

		stats.TradeCount++
		stats.Volume += trade.AmountSOL

		if trade.Action == "buy" {
			stats.BuyCount++
		} else if trade.Action == "sell" {
			stats.SellCount++
			stats.PnL += trade.PnL
		}
	}

	// Convert map to sorted slice
	var breakdown []HourlyStats
	for hour := 0; hour < 24; hour++ {
		if stats, exists := hourlyMap[hour]; exists {
			breakdown = append(breakdown, *stats)
		}
	}

	return breakdown
}
