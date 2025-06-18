package monitor

import (
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"go.uber.org/zap"
)

func TestPriceThrottlerConcurrentAccess(t *testing.T) {
	logger := zap.NewNop()
	outputCh := make(chan tea.Msg, 100)
	throttler := NewPriceThrottler(100*time.Millisecond, outputCh, logger)

	// Simulate concurrent price updates
	var wg sync.WaitGroup
	numGoroutines := 10
	updatesPerGoroutine := 100

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < updatesPerGoroutine; j++ {
				update := PriceUpdate{
					Current: float64(id*1000 + j),
					Initial: 100.0,
					Percent: float64(j),
					Tokens:  1000.0,
				}
				throttler.SendPriceUpdate(update)
			}
		}(i)
	}

	// Also test concurrent flush
	go func() {
		for i := 0; i < 50; i++ {
			throttler.FlushPending()
			time.Sleep(10 * time.Millisecond)
		}
	}()

	// Also test concurrent stats reading
	go func() {
		for i := 0; i < 50; i++ {
			sent, dropped := throttler.GetStats()
			_ = sent
			_ = dropped
			time.Sleep(10 * time.Millisecond)
		}
	}()

	wg.Wait()

	// Give some time for final operations
	time.Sleep(200 * time.Millisecond)

	sent, dropped := throttler.GetStats()
	t.Logf("Sent: %d, Dropped: %d", sent, dropped)

	// Should have some sent updates
	if sent == 0 {
		t.Error("Expected some updates to be sent")
	}

	// Check that total is reasonable
	// Note: We might get slightly more than expected due to flush operations
	// and race conditions in counting, but it should be close
	total := sent + dropped
	expected := uint64(numGoroutines * updatesPerGoroutine)
	tolerance := uint64(10) // Allow for small variance due to concurrent operations
	if total > expected+tolerance {
		t.Errorf("Total updates (%d) significantly exceeds expected (%d) with tolerance %d", total, expected, tolerance)
	}
}

func TestPriceThrottlerThrottling(t *testing.T) {
	logger := zap.NewNop()
	outputCh := make(chan tea.Msg, 10)
	throttler := NewPriceThrottler(50*time.Millisecond, outputCh, logger)

	// Send updates rapidly
	for i := 0; i < 5; i++ {
		update := PriceUpdate{
			Current: float64(100 + i),
			Initial: 100.0,
			Percent: float64(i),
			Tokens:  1000.0,
		}
		throttler.SendPriceUpdate(update)
		time.Sleep(10 * time.Millisecond) // Less than throttle interval
	}

	sent, dropped := throttler.GetStats()
	t.Logf("After rapid updates - Sent: %d, Dropped: %d", sent, dropped)

	// Should have throttled some updates
	if dropped == 0 {
		t.Error("Expected some updates to be throttled")
	}

	// Wait for throttle interval and flush
	time.Sleep(60 * time.Millisecond)
	throttler.FlushPending()

	sent2, _ := throttler.GetStats()
	if sent2 <= sent {
		t.Error("Expected flush to send pending update")
	}
}
