package monitor

import (
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// AlertType represents different types of alerts
type AlertType string

const (
	AlertTypePriceDrop    AlertType = "price_drop"
	AlertTypeProfitTarget AlertType = "profit_target"
	AlertTypeLossLimit    AlertType = "loss_limit"
	AlertTypeVolume       AlertType = "volume"
	AlertTypeStale        AlertType = "stale_position"
)

// Alert represents a triggered alert
type Alert struct {
	ID          string    `json:"id"`
	Type        AlertType `json:"type"`
	Timestamp   time.Time `json:"timestamp"`
	TokenMint   string    `json:"token_mint"`
	TokenSymbol string    `json:"token_symbol"`
	Message     string    `json:"message"`
	Details     string    `json:"details"`
	Severity    string    `json:"severity"` // "info", "warning", "critical"

	// Alert-specific data
	CurrentPrice float64 `json:"current_price,omitempty"`
	PnLPercent   float64 `json:"pnl_percent,omitempty"`
	Threshold    float64 `json:"threshold,omitempty"`
}

// AlertConfig holds alert configuration
type AlertConfig struct {
	// Price drop alert (negative percentage)
	PriceDropPercent float64 `json:"price_drop_percent"`

	// Profit target alert (positive percentage)
	ProfitTargetPercent float64 `json:"profit_target_percent"`

	// Loss limit alert (negative percentage)
	LossLimitPercent float64 `json:"loss_limit_percent"`

	// Volume alert (SOL amount)
	VolumeThreshold float64 `json:"volume_threshold"`

	// Stale position alert (duration)
	StalePositionDuration time.Duration `json:"stale_position_duration"`

	// Alert cooldown to prevent spam
	CooldownDuration time.Duration `json:"cooldown_duration"`
}

// DefaultAlertConfig returns default alert configuration
func DefaultAlertConfig() AlertConfig {
	return AlertConfig{
		PriceDropPercent:      10.0, // Alert on 10% drop
		ProfitTargetPercent:   50.0, // Alert on 50% profit
		LossLimitPercent:      20.0, // Alert on 20% loss
		VolumeThreshold:       10.0, // Alert on trades > 10 SOL
		StalePositionDuration: 1 * time.Hour,
		CooldownDuration:      5 * time.Minute,
	}
}

// AlertManager manages position alerts
type AlertManager struct {
	mu     sync.RWMutex
	config AlertConfig
	logger *zap.Logger

	// Track alerts
	alerts       []Alert
	maxAlerts    int
	alertHistory map[string]time.Time // token -> last alert time

	// Alert handlers
	handlers []AlertHandler
}

// AlertHandler is called when an alert is triggered
type AlertHandler func(alert Alert)

// NewAlertManager creates a new alert manager
func NewAlertManager(config AlertConfig, logger *zap.Logger) *AlertManager {
	return &AlertManager{
		config:       config,
		logger:       logger,
		alerts:       make([]Alert, 0, 100),
		maxAlerts:    1000,
		alertHistory: make(map[string]time.Time),
		handlers:     make([]AlertHandler, 0),
	}
}

// AddHandler adds an alert handler
func (am *AlertManager) AddHandler(handler AlertHandler) {
	am.mu.Lock()
	defer am.mu.Unlock()
	am.handlers = append(am.handlers, handler)
}

// CheckPosition checks a position for alerts
func (am *AlertManager) CheckPosition(pos Position) []Alert {
	am.mu.Lock()
	defer am.mu.Unlock()

	var triggered []Alert
	now := time.Now()

	// Check cooldown
	if lastAlert, exists := am.alertHistory[pos.TokenMint]; exists {
		if now.Sub(lastAlert) < am.config.CooldownDuration {
			return triggered // Skip due to cooldown
		}
	}

	// Price drop alert
	if am.config.PriceDropPercent > 0 && pos.PnLPercent < -am.config.PriceDropPercent {
		alert := Alert{
			ID:           fmt.Sprintf("alert_%d", now.UnixNano()),
			Type:         AlertTypePriceDrop,
			Timestamp:    now,
			TokenMint:    pos.TokenMint,
			TokenSymbol:  pos.TokenSymbol,
			Message:      fmt.Sprintf("Price dropped %.1f%% for %s", -pos.PnLPercent, pos.TokenSymbol),
			Details:      fmt.Sprintf("Current: %.6f SOL, Entry: %.6f SOL", pos.CurrentSOL, pos.InitialSOL),
			Severity:     "warning",
			CurrentPrice: pos.CurrentSOL,
			PnLPercent:   pos.PnLPercent,
			Threshold:    -am.config.PriceDropPercent,
		}
		triggered = append(triggered, alert)
		am.triggerAlert(alert)
	}

	// Profit target alert
	if am.config.ProfitTargetPercent > 0 && pos.PnLPercent >= am.config.ProfitTargetPercent {
		alert := Alert{
			ID:           fmt.Sprintf("alert_%d", now.UnixNano()),
			Type:         AlertTypeProfitTarget,
			Timestamp:    now,
			TokenMint:    pos.TokenMint,
			TokenSymbol:  pos.TokenSymbol,
			Message:      fmt.Sprintf("Profit target reached! +%.1f%% for %s", pos.PnLPercent, pos.TokenSymbol),
			Details:      fmt.Sprintf("Current: %.6f SOL, Entry: %.6f SOL", pos.CurrentSOL, pos.InitialSOL),
			Severity:     "info",
			CurrentPrice: pos.CurrentSOL,
			PnLPercent:   pos.PnLPercent,
			Threshold:    am.config.ProfitTargetPercent,
		}
		triggered = append(triggered, alert)
		am.triggerAlert(alert)
	}

	// Loss limit alert
	if am.config.LossLimitPercent > 0 && pos.PnLPercent < -am.config.LossLimitPercent {
		alert := Alert{
			ID:           fmt.Sprintf("alert_%d", now.UnixNano()),
			Type:         AlertTypeLossLimit,
			Timestamp:    now,
			TokenMint:    pos.TokenMint,
			TokenSymbol:  pos.TokenSymbol,
			Message:      fmt.Sprintf("LOSS LIMIT! -%.1f%% for %s", -pos.PnLPercent, pos.TokenSymbol),
			Details:      fmt.Sprintf("Consider selling. Current: %.6f SOL, Entry: %.6f SOL", pos.CurrentSOL, pos.InitialSOL),
			Severity:     "critical",
			CurrentPrice: pos.CurrentSOL,
			PnLPercent:   pos.PnLPercent,
			Threshold:    -am.config.LossLimitPercent,
		}
		triggered = append(triggered, alert)
		am.triggerAlert(alert)
	}

	// Stale position alert
	if am.config.StalePositionDuration > 0 && now.Sub(pos.UpdatedAt) > am.config.StalePositionDuration {
		alert := Alert{
			ID:          fmt.Sprintf("alert_%d", now.UnixNano()),
			Type:        AlertTypeStale,
			Timestamp:   now,
			TokenMint:   pos.TokenMint,
			TokenSymbol: pos.TokenSymbol,
			Message:     fmt.Sprintf("Stale position: %s not updated for %v", pos.TokenSymbol, now.Sub(pos.UpdatedAt)),
			Details:     "Consider checking the position manually",
			Severity:    "info",
		}
		triggered = append(triggered, alert)
		am.triggerAlert(alert)
	}

	// Update history if alerts were triggered
	if len(triggered) > 0 {
		am.alertHistory[pos.TokenMint] = now
	}

	return triggered
}

// CheckTrade checks a trade for volume alerts
func (am *AlertManager) CheckTrade(trade Trade) []Alert {
	am.mu.Lock()
	defer am.mu.Unlock()

	var triggered []Alert
	now := time.Now()

	// Volume alert
	if am.config.VolumeThreshold > 0 && trade.AmountSOL >= am.config.VolumeThreshold {
		alert := Alert{
			ID:          fmt.Sprintf("alert_%d", now.UnixNano()),
			Type:        AlertTypeVolume,
			Timestamp:   now,
			TokenMint:   trade.TokenMint,
			TokenSymbol: trade.TokenSymbol,
			Message:     fmt.Sprintf("Large %s: %.2f SOL for %s", trade.Action, trade.AmountSOL, trade.TokenSymbol),
			Details:     fmt.Sprintf("Transaction: %s", trade.TxSignature),
			Severity:    "info",
			Threshold:   am.config.VolumeThreshold,
		}
		triggered = append(triggered, alert)
		am.triggerAlert(alert)
	}

	return triggered
}

// triggerAlert handles alert triggering
func (am *AlertManager) triggerAlert(alert Alert) {
	// Add to history
	if len(am.alerts) >= am.maxAlerts {
		am.alerts = am.alerts[1:]
	}
	am.alerts = append(am.alerts, alert)

	// Log alert
	switch alert.Severity {
	case "critical":
		am.logger.Error("Alert triggered",
			zap.String("type", string(alert.Type)),
			zap.String("token", alert.TokenSymbol),
			zap.String("message", alert.Message))
	case "warning":
		am.logger.Warn("Alert triggered",
			zap.String("type", string(alert.Type)),
			zap.String("token", alert.TokenSymbol),
			zap.String("message", alert.Message))
	default:
		am.logger.Info("Alert triggered",
			zap.String("type", string(alert.Type)),
			zap.String("token", alert.TokenSymbol),
			zap.String("message", alert.Message))
	}

	// Call handlers
	for _, handler := range am.handlers {
		go handler(alert)
	}
}

// GetRecentAlerts returns recent alerts
func (am *AlertManager) GetRecentAlerts(limit int) []Alert {
	am.mu.RLock()
	defer am.mu.RUnlock()

	if limit <= 0 || limit > len(am.alerts) {
		limit = len(am.alerts)
	}

	start := len(am.alerts) - limit
	if start < 0 {
		start = 0
	}

	result := make([]Alert, limit)
	copy(result, am.alerts[start:])

	return result
}

// GetAlertsByToken returns alerts for a specific token
func (am *AlertManager) GetAlertsByToken(tokenMint string) []Alert {
	am.mu.RLock()
	defer am.mu.RUnlock()

	var result []Alert
	for _, alert := range am.alerts {
		if alert.TokenMint == tokenMint {
			result = append(result, alert)
		}
	}

	return result
}

// UpdateConfig updates the alert configuration
func (am *AlertManager) UpdateConfig(config AlertConfig) {
	am.mu.Lock()
	defer am.mu.Unlock()

	am.config = config
	am.logger.Info("Alert configuration updated",
		zap.Float64("price_drop_percent", config.PriceDropPercent),
		zap.Float64("profit_target_percent", config.ProfitTargetPercent),
		zap.Float64("loss_limit_percent", config.LossLimitPercent),
		zap.Float64("volume_threshold", config.VolumeThreshold))
}

// GetConfig returns the current alert configuration
func (am *AlertManager) GetConfig() AlertConfig {
	am.mu.RLock()
	defer am.mu.RUnlock()
	return am.config
}

// ClearHistory clears the alert cooldown history
func (am *AlertManager) ClearHistory() {
	am.mu.Lock()
	defer am.mu.Unlock()
	am.alertHistory = make(map[string]time.Time)
}
