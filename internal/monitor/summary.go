package monitor

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"
)

// SummaryGenerator generates performance summaries
type SummaryGenerator struct {
	logger *zap.Logger
}

// NewSummaryGenerator creates a new summary generator
func NewSummaryGenerator(logger *zap.Logger) *SummaryGenerator {
	return &SummaryGenerator{
		logger: logger,
	}
}

// PerformanceSummary represents a performance summary
type PerformanceSummary struct {
	Period           string         `json:"period"`
	StartTime        time.Time      `json:"start_time"`
	EndTime          time.Time      `json:"end_time"`
	Overview         OverviewStats  `json:"overview"`
	TradingMetrics   TradingMetrics `json:"trading_metrics"`
	TokenPerformance []TokenStats   `json:"token_performance"`
	TimeAnalysis     TimeAnalysis   `json:"time_analysis"`
	RiskMetrics      RiskMetrics    `json:"risk_metrics"`
	TopTrades        []Trade        `json:"top_trades"`
	Recommendations  []string       `json:"recommendations"`
}

// OverviewStats contains high-level statistics
type OverviewStats struct {
	TotalTrades   int     `json:"total_trades"`
	SuccessRate   float64 `json:"success_rate"`
	TotalVolume   float64 `json:"total_volume"`
	NetPnL        float64 `json:"net_pnl"`
	NetPnLPercent float64 `json:"net_pnl_percent"`
	ActiveTokens  int     `json:"active_tokens"`
	TradingHours  float64 `json:"trading_hours"`
}

// TradingMetrics contains detailed trading metrics
type TradingMetrics struct {
	TotalBuys     int     `json:"total_buys"`
	TotalSells    int     `json:"total_sells"`
	WinningTrades int     `json:"winning_trades"`
	LosingTrades  int     `json:"losing_trades"`
	WinRate       float64 `json:"win_rate"`
	AvgWinAmount  float64 `json:"avg_win_amount"`
	AvgLossAmount float64 `json:"avg_loss_amount"`
	LargestWin    float64 `json:"largest_win"`
	LargestLoss   float64 `json:"largest_loss"`
	AvgHoldTime   string  `json:"avg_hold_time"`
	ProfitFactor  float64 `json:"profit_factor"`
}

// TokenStats contains performance statistics for a token
type TokenStats struct {
	TokenMint   string  `json:"token_mint"`
	TokenSymbol string  `json:"token_symbol"`
	TradeCount  int     `json:"trade_count"`
	Volume      float64 `json:"volume"`
	PnL         float64 `json:"pnl"`
	PnLPercent  float64 `json:"pnl_percent"`
	WinRate     float64 `json:"win_rate"`
	AvgHoldTime string  `json:"avg_hold_time"`
}

// TimeAnalysis contains time-based analysis
type TimeAnalysis struct {
	MostActiveHour     int             `json:"most_active_hour"`
	MostProfitableHour int             `json:"most_profitable_hour"`
	HourlyActivity     map[int]int     `json:"hourly_activity"`
	HourlyPnL          map[int]float64 `json:"hourly_pnl"`
}

// RiskMetrics contains risk-related metrics
type RiskMetrics struct {
	MaxDrawdown        float64 `json:"max_drawdown"`
	MaxDrawdownPercent float64 `json:"max_drawdown_percent"`
	ConsecutiveWins    int     `json:"consecutive_wins"`
	ConsecutiveLosses  int     `json:"consecutive_losses"`
	RiskRewardRatio    float64 `json:"risk_reward_ratio"`
	SharpeRatio        float64 `json:"sharpe_ratio"`
}

// GenerateSummary generates a performance summary
func (sg *SummaryGenerator) GenerateSummary(trades []Trade, period string, startTime, endTime time.Time) *PerformanceSummary {
	if len(trades) == 0 {
		return &PerformanceSummary{
			Period:    period,
			StartTime: startTime,
			EndTime:   endTime,
		}
	}

	summary := &PerformanceSummary{
		Period:    period,
		StartTime: startTime,
		EndTime:   endTime,
	}

	// Filter trades by time period
	var filteredTrades []Trade
	for _, trade := range trades {
		if trade.Timestamp.After(startTime) && trade.Timestamp.Before(endTime) {
			filteredTrades = append(filteredTrades, trade)
		}
	}

	if len(filteredTrades) == 0 {
		return summary
	}

	// Calculate overview stats
	summary.Overview = sg.calculateOverview(filteredTrades, startTime, endTime)

	// Calculate trading metrics
	summary.TradingMetrics = sg.calculateTradingMetrics(filteredTrades)

	// Calculate token performance
	summary.TokenPerformance = sg.calculateTokenPerformance(filteredTrades)

	// Calculate time analysis
	summary.TimeAnalysis = sg.calculateTimeAnalysis(filteredTrades)

	// Calculate risk metrics
	summary.RiskMetrics = sg.calculateRiskMetrics(filteredTrades)

	// Get top trades
	summary.TopTrades = sg.getTopTrades(filteredTrades, 5)

	// Generate recommendations
	summary.Recommendations = sg.generateRecommendations(summary)

	return summary
}

// calculateOverview calculates overview statistics
func (sg *SummaryGenerator) calculateOverview(trades []Trade, startTime, endTime time.Time) OverviewStats {
	overview := OverviewStats{
		TotalTrades:  len(trades),
		TradingHours: endTime.Sub(startTime).Hours(),
	}

	tokenSet := make(map[string]bool)
	successCount := 0

	for _, trade := range trades {
		tokenSet[trade.TokenMint] = true

		if trade.Success {
			successCount++
		}

		overview.TotalVolume += trade.AmountSOL
		overview.NetPnL += trade.PnL
	}

	overview.ActiveTokens = len(tokenSet)

	if overview.TotalTrades > 0 {
		overview.SuccessRate = float64(successCount) / float64(overview.TotalTrades) * 100
	}

	if overview.TotalVolume > 0 {
		overview.NetPnLPercent = overview.NetPnL / overview.TotalVolume * 100
	}

	return overview
}

// calculateTradingMetrics calculates detailed trading metrics
func (sg *SummaryGenerator) calculateTradingMetrics(trades []Trade) TradingMetrics {
	metrics := TradingMetrics{}

	var (
		totalWins     float64
		totalLosses   float64
		holdDurations []time.Duration
		buyTimes      = make(map[string]time.Time)
	)

	for _, trade := range trades {
		if trade.Action == "buy" {
			metrics.TotalBuys++
			buyTimes[trade.TokenMint] = trade.Timestamp
		} else if trade.Action == "sell" {
			metrics.TotalSells++

			// Calculate hold time
			if buyTime, exists := buyTimes[trade.TokenMint]; exists {
				holdDurations = append(holdDurations, trade.Timestamp.Sub(buyTime))
			}

			// Track wins/losses
			if trade.PnL > 0 {
				metrics.WinningTrades++
				totalWins += trade.PnL
				if trade.PnL > metrics.LargestWin {
					metrics.LargestWin = trade.PnL
				}
			} else if trade.PnL < 0 {
				metrics.LosingTrades++
				totalLosses += -trade.PnL
				if trade.PnL < metrics.LargestLoss {
					metrics.LargestLoss = trade.PnL
				}
			}
		}
	}

	// Calculate averages
	if metrics.TotalSells > 0 {
		metrics.WinRate = float64(metrics.WinningTrades) / float64(metrics.TotalSells) * 100
	}

	if metrics.WinningTrades > 0 {
		metrics.AvgWinAmount = totalWins / float64(metrics.WinningTrades)
	}

	if metrics.LosingTrades > 0 {
		metrics.AvgLossAmount = totalLosses / float64(metrics.LosingTrades)
	}

	if totalLosses > 0 {
		metrics.ProfitFactor = totalWins / totalLosses
	}

	// Calculate average hold time
	if len(holdDurations) > 0 {
		var totalDuration time.Duration
		for _, d := range holdDurations {
			totalDuration += d
		}
		avgDuration := totalDuration / time.Duration(len(holdDurations))
		metrics.AvgHoldTime = formatDuration(avgDuration)
	}

	return metrics
}

// calculateTokenPerformance calculates performance by token
func (sg *SummaryGenerator) calculateTokenPerformance(trades []Trade) []TokenStats {
	tokenMap := make(map[string]*TokenStats)
	tokenBuyTimes := make(map[string][]time.Time)
	tokenSellTimes := make(map[string][]time.Time)

	for _, trade := range trades {
		stats, exists := tokenMap[trade.TokenMint]
		if !exists {
			stats = &TokenStats{
				TokenMint:   trade.TokenMint,
				TokenSymbol: trade.TokenSymbol,
			}
			tokenMap[trade.TokenMint] = stats
		}

		stats.TradeCount++
		stats.Volume += trade.AmountSOL

		if trade.Action == "buy" {
			tokenBuyTimes[trade.TokenMint] = append(tokenBuyTimes[trade.TokenMint], trade.Timestamp)
		} else if trade.Action == "sell" {
			tokenSellTimes[trade.TokenMint] = append(tokenSellTimes[trade.TokenMint], trade.Timestamp)
			stats.PnL += trade.PnL
		}
	}

	// Calculate additional metrics
	for tokenMint, stats := range tokenMap {
		if stats.Volume > 0 {
			stats.PnLPercent = stats.PnL / stats.Volume * 100
		}

		// Calculate average hold time
		buyTimes := tokenBuyTimes[tokenMint]
		sellTimes := tokenSellTimes[tokenMint]

		if len(buyTimes) > 0 && len(sellTimes) > 0 {
			var totalDuration time.Duration
			minLen := len(buyTimes)
			if len(sellTimes) < minLen {
				minLen = len(sellTimes)
			}

			for i := 0; i < minLen; i++ {
				totalDuration += sellTimes[i].Sub(buyTimes[i])
			}

			if minLen > 0 {
				avgDuration := totalDuration / time.Duration(minLen)
				stats.AvgHoldTime = formatDuration(avgDuration)
			}
		}
	}

	// Convert map to slice and sort by PnL
	var tokenStats []TokenStats
	for _, stats := range tokenMap {
		tokenStats = append(tokenStats, *stats)
	}

	sort.Slice(tokenStats, func(i, j int) bool {
		return tokenStats[i].PnL > tokenStats[j].PnL
	})

	return tokenStats
}

// calculateTimeAnalysis analyzes trading patterns by time
func (sg *SummaryGenerator) calculateTimeAnalysis(trades []Trade) TimeAnalysis {
	analysis := TimeAnalysis{
		HourlyActivity: make(map[int]int),
		HourlyPnL:      make(map[int]float64),
	}

	for _, trade := range trades {
		hour := trade.Timestamp.Hour()
		analysis.HourlyActivity[hour]++

		if trade.Action == "sell" {
			analysis.HourlyPnL[hour] += trade.PnL
		}
	}

	// Find most active hour
	maxActivity := 0
	for hour, count := range analysis.HourlyActivity {
		if count > maxActivity {
			maxActivity = count
			analysis.MostActiveHour = hour
		}
	}

	// Find most profitable hour
	maxPnL := -999999.0
	for hour, pnl := range analysis.HourlyPnL {
		if pnl > maxPnL {
			maxPnL = pnl
			analysis.MostProfitableHour = hour
		}
	}

	return analysis
}

// calculateRiskMetrics calculates risk-related metrics
func (sg *SummaryGenerator) calculateRiskMetrics(trades []Trade) RiskMetrics {
	metrics := RiskMetrics{}

	var (
		currentDrawdown float64
		maxDrawdown     float64
		peakValue       float64
		currentStreak   int
		maxWinStreak    int
		maxLossStreak   int
		lastWasWin      bool
		returns         []float64
	)

	cumPnL := 0.0
	for _, trade := range trades {
		if trade.Action == "sell" {
			cumPnL += trade.PnL

			// Track peak value
			if cumPnL > peakValue {
				peakValue = cumPnL
			}

			// Calculate drawdown
			if peakValue > 0 {
				currentDrawdown = peakValue - cumPnL
				if currentDrawdown > maxDrawdown {
					maxDrawdown = currentDrawdown
				}
			}

			// Track streaks
			isWin := trade.PnL > 0
			if isWin == lastWasWin {
				currentStreak++
			} else {
				currentStreak = 1
				lastWasWin = isWin
			}

			if isWin && currentStreak > maxWinStreak {
				maxWinStreak = currentStreak
			} else if !isWin && currentStreak > maxLossStreak {
				maxLossStreak = currentStreak
			}

			// Collect returns for Sharpe ratio
			if trade.AmountSOL > 0 {
				returns = append(returns, trade.PnL/trade.AmountSOL)
			}
		}
	}

	metrics.MaxDrawdown = maxDrawdown
	if peakValue > 0 {
		metrics.MaxDrawdownPercent = maxDrawdown / peakValue * 100
	}

	metrics.ConsecutiveWins = maxWinStreak
	metrics.ConsecutiveLosses = maxLossStreak

	// Calculate Sharpe ratio (simplified)
	if len(returns) > 1 {
		avgReturn := 0.0
		for _, r := range returns {
			avgReturn += r
		}
		avgReturn /= float64(len(returns))

		// Calculate standard deviation
		variance := 0.0
		for _, r := range returns {
			variance += (r - avgReturn) * (r - avgReturn)
		}
		variance /= float64(len(returns) - 1)
		stdDev := sqrt(variance)

		if stdDev > 0 {
			// Annualized Sharpe ratio (assuming daily trading)
			metrics.SharpeRatio = avgReturn / stdDev * sqrt(252)
		}
	}

	return metrics
}

// getTopTrades returns the top performing trades
func (sg *SummaryGenerator) getTopTrades(trades []Trade, limit int) []Trade {
	// Filter sells only
	var sells []Trade
	for _, trade := range trades {
		if trade.Action == "sell" {
			sells = append(sells, trade)
		}
	}

	// Sort by PnL
	sort.Slice(sells, func(i, j int) bool {
		return sells[i].PnL > sells[j].PnL
	})

	// Return top trades
	if len(sells) < limit {
		limit = len(sells)
	}

	return sells[:limit]
}

// generateRecommendations generates trading recommendations based on the summary
func (sg *SummaryGenerator) generateRecommendations(summary *PerformanceSummary) []string {
	var recommendations []string

	// Win rate recommendations
	if summary.TradingMetrics.WinRate < 40 {
		recommendations = append(recommendations,
			"⚠️ Low win rate detected. Consider reviewing entry criteria and risk management.")
	} else if summary.TradingMetrics.WinRate > 70 {
		recommendations = append(recommendations,
			"✅ Excellent win rate! Consider increasing position sizes gradually.")
	}

	// Risk-reward recommendations
	if summary.TradingMetrics.AvgLossAmount > summary.TradingMetrics.AvgWinAmount {
		recommendations = append(recommendations,
			"⚠️ Average losses exceed average wins. Consider tightening stop losses or targeting higher profits.")
	}

	// Drawdown recommendations
	if summary.RiskMetrics.MaxDrawdownPercent > 20 {
		recommendations = append(recommendations,
			"🚨 High drawdown detected. Consider reducing position sizes to manage risk.")
	}

	// Streak recommendations
	if summary.RiskMetrics.ConsecutiveLosses > 5 {
		recommendations = append(recommendations,
			"⚠️ Long losing streak detected. Take a break and review your strategy.")
	}

	// Time-based recommendations
	if summary.TimeAnalysis.MostProfitableHour != summary.TimeAnalysis.MostActiveHour {
		recommendations = append(recommendations,
			fmt.Sprintf("💡 Most profitable hour (%d:00) differs from most active hour (%d:00). Consider focusing trading during profitable hours.",
				summary.TimeAnalysis.MostProfitableHour, summary.TimeAnalysis.MostActiveHour))
	}

	// Token diversity recommendations
	if summary.Overview.ActiveTokens < 5 && summary.Overview.TotalTrades > 20 {
		recommendations = append(recommendations,
			"📊 Low token diversity. Consider expanding to more tokens to spread risk.")
	}

	return recommendations
}

// Helper functions

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	} else if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	} else if d < 24*time.Hour {
		return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
	}
	days := int(d.Hours() / 24)
	hours := int(d.Hours()) % 24
	return fmt.Sprintf("%dd%dh", days, hours)
}

func sqrt(x float64) float64 {
	if x < 0 {
		return 0
	}
	z := x
	for i := 0; i < 10; i++ {
		z = (z + x/z) / 2
	}
	return z
}

// GenerateDailySummary generates a summary for the current day
func (sg *SummaryGenerator) GenerateDailySummary(trades []Trade) *PerformanceSummary {
	now := time.Now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)

	return sg.GenerateSummary(trades, "Daily", startOfDay, endOfDay)
}

// FormatSummaryText formats a summary as human-readable text
func (sg *SummaryGenerator) FormatSummaryText(summary *PerformanceSummary) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("📊 %s Performance Summary\n", summary.Period))
	sb.WriteString(fmt.Sprintf("Period: %s - %s\n",
		summary.StartTime.Format("2006-01-02 15:04"),
		summary.EndTime.Format("2006-01-02 15:04")))
	sb.WriteString(strings.Repeat("=", 50) + "\n\n")

	// Overview
	sb.WriteString("📈 Overview\n")
	sb.WriteString(fmt.Sprintf("• Total Trades: %d\n", summary.Overview.TotalTrades))
	sb.WriteString(fmt.Sprintf("• Success Rate: %.1f%%\n", summary.Overview.SuccessRate))
	sb.WriteString(fmt.Sprintf("• Total Volume: %.2f SOL\n", summary.Overview.TotalVolume))
	sb.WriteString(fmt.Sprintf("• Net P&L: %.4f SOL (%.1f%%)\n",
		summary.Overview.NetPnL, summary.Overview.NetPnLPercent))
	sb.WriteString(fmt.Sprintf("• Active Tokens: %d\n\n", summary.Overview.ActiveTokens))

	// Trading Metrics
	sb.WriteString("💰 Trading Metrics\n")
	sb.WriteString(fmt.Sprintf("• Buys/Sells: %d/%d\n",
		summary.TradingMetrics.TotalBuys, summary.TradingMetrics.TotalSells))
	sb.WriteString(fmt.Sprintf("• Win Rate: %.1f%% (%d wins, %d losses)\n",
		summary.TradingMetrics.WinRate,
		summary.TradingMetrics.WinningTrades,
		summary.TradingMetrics.LosingTrades))
	sb.WriteString(fmt.Sprintf("• Avg Win/Loss: %.4f / %.4f SOL\n",
		summary.TradingMetrics.AvgWinAmount,
		summary.TradingMetrics.AvgLossAmount))
	sb.WriteString(fmt.Sprintf("• Profit Factor: %.2f\n", summary.TradingMetrics.ProfitFactor))
	sb.WriteString(fmt.Sprintf("• Avg Hold Time: %s\n\n", summary.TradingMetrics.AvgHoldTime))

	// Top Performers
	if len(summary.TokenPerformance) > 0 {
		sb.WriteString("🏆 Top Performing Tokens\n")
		for i, token := range summary.TokenPerformance {
			if i >= 3 {
				break
			}
			sb.WriteString(fmt.Sprintf("%d. %s: %.4f SOL (%.1f%%)\n",
				i+1, token.TokenSymbol, token.PnL, token.PnLPercent))
		}
		sb.WriteString("\n")
	}

	// Risk Metrics
	sb.WriteString("⚠️ Risk Metrics\n")
	sb.WriteString(fmt.Sprintf("• Max Drawdown: %.4f SOL (%.1f%%)\n",
		summary.RiskMetrics.MaxDrawdown,
		summary.RiskMetrics.MaxDrawdownPercent))
	sb.WriteString(fmt.Sprintf("• Consecutive Wins/Losses: %d/%d\n",
		summary.RiskMetrics.ConsecutiveWins,
		summary.RiskMetrics.ConsecutiveLosses))
	if summary.RiskMetrics.SharpeRatio != 0 {
		sb.WriteString(fmt.Sprintf("• Sharpe Ratio: %.2f\n", summary.RiskMetrics.SharpeRatio))
	}
	sb.WriteString("\n")

	// Recommendations
	if len(summary.Recommendations) > 0 {
		sb.WriteString("💡 Recommendations\n")
		for _, rec := range summary.Recommendations {
			sb.WriteString(fmt.Sprintf("• %s\n", rec))
		}
	}

	return sb.String()
}
