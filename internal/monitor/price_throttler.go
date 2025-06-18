package monitor

import (
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"go.uber.org/zap"
)

// PriceThrottler provides thread-safe price update throttling to prevent
// overwhelming the UI with updates and avoid race conditions.
type PriceThrottler struct {
	mu             sync.RWMutex
	updateInterval time.Duration
	lastUpdate     time.Time
	pendingUpdate  *PriceUpdate
	outputCh       chan tea.Msg
	logger         *zap.Logger

	// Stats for monitoring
	droppedUpdates uint64
	sentUpdates    uint64
}

// NewPriceThrottler creates a new price throttler with the specified update interval.
func NewPriceThrottler(updateInterval time.Duration, outputCh chan tea.Msg, logger *zap.Logger) *PriceThrottler {
	return &PriceThrottler{
		updateInterval: updateInterval,
		outputCh:       outputCh,
		logger:         logger,
		lastUpdate:     time.Time{}, // Zero time initially
	}
}

// SendPriceUpdate safely sends a price update, throttling if necessary.
// This method is thread-safe and can be called from multiple goroutines.
func (pt *PriceThrottler) SendPriceUpdate(update PriceUpdate) {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	now := time.Now()

	// Check if enough time has passed since the last update
	if now.Sub(pt.lastUpdate) < pt.updateInterval {
		// Store the update as pending
		pt.pendingUpdate = &update
		pt.droppedUpdates++
		pt.logger.Debug("Price update throttled",
			zap.Float64("price", update.Current),
			zap.Duration("timeSinceLastUpdate", now.Sub(pt.lastUpdate)))
		return
	}

	// Send the update
	select {
	case pt.outputCh <- update:
		pt.lastUpdate = now
		pt.sentUpdates++
		pt.pendingUpdate = nil // Clear any pending update
		pt.logger.Debug("Price update sent",
			zap.Float64("price", update.Current),
			zap.Float64("percent", update.Percent))
	default:
		// Channel is full, store as pending
		pt.pendingUpdate = &update
		pt.droppedUpdates++
		pt.logger.Warn("Price update channel full, storing as pending",
			zap.Float64("price", update.Current))
	}
}

// FlushPending sends any pending update if enough time has passed.
// This should be called periodically to ensure pending updates are eventually sent.
func (pt *PriceThrottler) FlushPending() {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	if pt.pendingUpdate == nil {
		return
	}

	now := time.Now()
	if now.Sub(pt.lastUpdate) >= pt.updateInterval {
		select {
		case pt.outputCh <- *pt.pendingUpdate:
			pt.lastUpdate = now
			pt.sentUpdates++
			pt.logger.Debug("Pending price update flushed",
				zap.Float64("price", pt.pendingUpdate.Current))
			pt.pendingUpdate = nil
		default:
			// Still can't send, keep as pending
			pt.logger.Debug("Cannot flush pending update, channel still full")
		}
	}
}

// GetStats returns statistics about the throttler's operation.
func (pt *PriceThrottler) GetStats() (sent, dropped uint64) {
	pt.mu.RLock()
	defer pt.mu.RUnlock()
	return pt.sentUpdates, pt.droppedUpdates
}

// GetLastUpdate returns the last update time.
func (pt *PriceThrottler) GetLastUpdate() time.Time {
	pt.mu.RLock()
	defer pt.mu.RUnlock()
	return pt.lastUpdate
}

// HasPendingUpdate returns true if there's a pending update.
func (pt *PriceThrottler) HasPendingUpdate() bool {
	pt.mu.RLock()
	defer pt.mu.RUnlock()
	return pt.pendingUpdate != nil
}
