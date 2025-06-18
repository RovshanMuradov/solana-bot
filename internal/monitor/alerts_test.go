package monitor

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestAlertManagerConcurrentAccess(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultAlertConfig()
	config.CooldownDuration = 100 * time.Millisecond // Short cooldown for testing

	alertManager := NewAlertManager(config, logger)

	// Track alerts
	var alertCount int32

	alertManager.AddHandler(func(alert Alert) {
		atomic.AddInt32(&alertCount, 1)
	})

	var wg sync.WaitGroup
	numGoroutines := 10

	// Concurrent position checks
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				pos := Position{
					TokenMint:   fmt.Sprintf("token_%d", id),
					TokenSymbol: fmt.Sprintf("TKN%d", id),
					InitialSOL:  1.0,
					CurrentSOL:  float64(100-j) / 100.0, // Price drops over time
					PnL:         float64(j) / -100.0,
					PnLPercent:  float64(-j),
					UpdatedAt:   time.Now(),
				}

				alerts := alertManager.CheckPosition(pos)
				_ = alerts

				time.Sleep(5 * time.Millisecond)
			}
		}(i)
	}

	// Concurrent trade checks
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				trade := Trade{
					TokenMint:   fmt.Sprintf("token_%d", id),
					TokenSymbol: fmt.Sprintf("TKN%d", id),
					Action:      "buy",
					AmountSOL:   float64(j) * 2, // Increasing volume
					TxSignature: fmt.Sprintf("sig_%d_%d", id, j),
				}

				alerts := alertManager.CheckTrade(trade)
				_ = alerts

				time.Sleep(10 * time.Millisecond)
			}
		}(i)
	}

	// Concurrent alert reads
	wg.Add(5)
	for i := 0; i < 5; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				recent := alertManager.GetRecentAlerts(10)
				_ = recent

				tokenAlerts := alertManager.GetAlertsByToken("token_0")
				_ = tokenAlerts

				time.Sleep(10 * time.Millisecond)
			}
		}()
	}

	wg.Wait()

	// Check results
	totalAlerts := atomic.LoadInt32(&alertCount)

	t.Logf("Total alerts triggered: %d", totalAlerts)

	if totalAlerts == 0 {
		t.Error("Expected some alerts to be triggered")
	}

	recent := alertManager.GetRecentAlerts(100)
	t.Logf("Recent alerts count: %d", len(recent))
}

func TestAlertTypes(t *testing.T) {
	logger := zap.NewNop()
	config := AlertConfig{
		PriceDropPercent:      10.0,
		ProfitTargetPercent:   50.0,
		LossLimitPercent:      20.0,
		VolumeThreshold:       10.0,
		StalePositionDuration: 1 * time.Hour,
		CooldownDuration:      0, // No cooldown for testing
	}

	alertManager := NewAlertManager(config, logger)

	var triggeredAlerts []Alert
	var mu sync.Mutex
	alertManager.AddHandler(func(alert Alert) {
		mu.Lock()
		triggeredAlerts = append(triggeredAlerts, alert)
		mu.Unlock()
	})

	// Test price drop alert
	pos1 := Position{
		TokenMint:   "token1",
		TokenSymbol: "TKN1",
		InitialSOL:  1.0,
		CurrentSOL:  0.85, // 15% drop
		PnL:         -0.15,
		PnLPercent:  -15,
		UpdatedAt:   time.Now(),
	}
	alerts := alertManager.CheckPosition(pos1)
	if len(alerts) != 1 || alerts[0].Type != AlertTypePriceDrop {
		t.Error("Expected price drop alert")
	}

	// Test profit target alert
	pos2 := Position{
		TokenMint:   "token2",
		TokenSymbol: "TKN2",
		InitialSOL:  1.0,
		CurrentSOL:  1.6, // 60% gain
		PnL:         0.6,
		PnLPercent:  60,
		UpdatedAt:   time.Now(),
	}
	alerts = alertManager.CheckPosition(pos2)
	if len(alerts) != 1 || alerts[0].Type != AlertTypeProfitTarget {
		t.Error("Expected profit target alert")
	}

	// Test loss limit alert
	pos3 := Position{
		TokenMint:   "token3",
		TokenSymbol: "TKN3",
		InitialSOL:  1.0,
		CurrentSOL:  0.75, // 25% loss
		PnL:         -0.25,
		PnLPercent:  -25,
		UpdatedAt:   time.Now(),
	}
	alerts = alertManager.CheckPosition(pos3)
	// Should get both price drop and loss limit alerts
	if len(alerts) != 2 {
		t.Errorf("Expected 2 alerts, got %d", len(alerts))
	}

	// Test volume alert
	trade := Trade{
		TokenMint:   "token4",
		TokenSymbol: "TKN4",
		Action:      "buy",
		AmountSOL:   15.0, // Above threshold
		TxSignature: "sig4",
	}
	alerts = alertManager.CheckTrade(trade)
	if len(alerts) != 1 || alerts[0].Type != AlertTypeVolume {
		t.Error("Expected volume alert")
	}

	// Test stale position alert
	pos4 := Position{
		TokenMint:   "token5",
		TokenSymbol: "TKN5",
		InitialSOL:  1.0,
		CurrentSOL:  1.0,
		UpdatedAt:   time.Now().Add(-2 * time.Hour), // 2 hours old
	}
	alerts = alertManager.CheckPosition(pos4)
	if len(alerts) != 1 || alerts[0].Type != AlertTypeStale {
		t.Error("Expected stale position alert")
	}

	// Wait a bit for handlers to complete
	time.Sleep(50 * time.Millisecond)

	// Verify all alerts were triggered
	mu.Lock()
	alertCount := len(triggeredAlerts)
	mu.Unlock()

	// We expect 5 alerts: price drop, profit target, price drop + loss limit (2), volume, stale
	if alertCount < 5 {
		t.Errorf("Expected at least 5 alerts, got %d", alertCount)
	}
}

func TestAlertCooldown(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultAlertConfig()
	config.CooldownDuration = 100 * time.Millisecond
	config.PriceDropPercent = 5.0

	alertManager := NewAlertManager(config, logger)

	pos := Position{
		TokenMint:   "token1",
		TokenSymbol: "TKN1",
		InitialSOL:  1.0,
		CurrentSOL:  0.9, // 10% drop
		PnL:         -0.1,
		PnLPercent:  -10,
		UpdatedAt:   time.Now(),
	}

	// First check should trigger alert
	alerts := alertManager.CheckPosition(pos)
	if len(alerts) != 1 {
		t.Error("Expected first alert to trigger")
	}

	// Immediate second check should not trigger due to cooldown
	alerts = alertManager.CheckPosition(pos)
	if len(alerts) != 0 {
		t.Error("Expected no alerts due to cooldown")
	}

	// Wait for cooldown to expire
	time.Sleep(150 * time.Millisecond)

	// Now alert should trigger again
	alerts = alertManager.CheckPosition(pos)
	if len(alerts) != 1 {
		t.Error("Expected alert after cooldown")
	}
}

func TestAlertConfigUpdate(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultAlertConfig()

	alertManager := NewAlertManager(config, logger)

	// Update config
	newConfig := AlertConfig{
		PriceDropPercent:    5.0,  // More sensitive
		ProfitTargetPercent: 20.0, // Lower target
		LossLimitPercent:    10.0, // Tighter limit
		VolumeThreshold:     5.0,  // Lower threshold
		CooldownDuration:    0,
	}

	alertManager.UpdateConfig(newConfig)

	// Verify config was updated
	currentConfig := alertManager.GetConfig()
	if currentConfig.PriceDropPercent != 5.0 {
		t.Errorf("Expected price drop percent 5.0, got %.1f", currentConfig.PriceDropPercent)
	}

	// Test with new thresholds
	pos := Position{
		TokenMint:   "token1",
		TokenSymbol: "TKN1",
		InitialSOL:  1.0,
		CurrentSOL:  0.93, // 7% drop (would not trigger with old config)
		PnL:         -0.07,
		PnLPercent:  -7,
		UpdatedAt:   time.Now(),
	}

	alerts := alertManager.CheckPosition(pos)
	if len(alerts) != 1 || alerts[0].Type != AlertTypePriceDrop {
		t.Error("Expected price drop alert with new config")
	}
}
