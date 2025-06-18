package state

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/rovshanmuradov/solana-bot/internal/monitor"
	"go.uber.org/zap"
)

// UICache provides thread-safe UI state caching
type UICache struct {
	positions map[string]monitor.Position
	mu        sync.RWMutex
	logger    *zap.Logger

	// Statistics (accessed atomically)
	reads  uint64
	writes uint64
}

// NewUICache creates a new UI state cache
func NewUICache(logger *zap.Logger) *UICache {
	return &UICache{
		positions: make(map[string]monitor.Position),
		logger:    logger,
	}
}

// UpdatePosition updates a position in the cache
func (c *UICache) UpdatePosition(sessionID string, update monitor.PriceUpdate) {
	c.mu.Lock()
	defer c.mu.Unlock()

	pos := c.positions[sessionID]
	pos.SessionID = sessionID
	pos.CurrentSOL = update.Current
	pos.PnL = update.Current - update.Initial
	pos.PnLPercent = update.Percent
	pos.UpdatedAt = time.Now()

	c.positions[sessionID] = pos
	atomic.AddUint64(&c.writes, 1)
}

// SetPosition sets a complete position in the cache
func (c *UICache) SetPosition(pos monitor.Position) {
	c.mu.Lock()
	defer c.mu.Unlock()

	pos.UpdatedAt = time.Now()
	c.positions[pos.SessionID] = pos
	atomic.AddUint64(&c.writes, 1)
}

// GetPosition returns a copy of a specific position
func (c *UICache) GetPosition(sessionID string) (monitor.Position, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	atomic.AddUint64(&c.reads, 1)
	pos, exists := c.positions[sessionID]
	return pos, exists
}

// GetSnapshot returns a copy of all positions
func (c *UICache) GetSnapshot() []monitor.Position {
	c.mu.RLock()
	defer c.mu.RUnlock()

	atomic.AddUint64(&c.reads, 1)
	// Return copy, not reference
	snapshot := make([]monitor.Position, 0, len(c.positions))
	for _, pos := range c.positions {
		snapshot = append(snapshot, pos)
	}

	return snapshot
}

// GetActivePositions returns positions with active monitoring
func (c *UICache) GetActivePositions() []monitor.Position {
	c.mu.RLock()
	defer c.mu.RUnlock()

	atomic.AddUint64(&c.reads, 1)
	active := make([]monitor.Position, 0)
	for _, pos := range c.positions {
		if pos.Status == "monitoring" || pos.Status == "active" {
			active = append(active, pos)
		}
	}

	return active
}

// RemovePosition removes a position from the cache
func (c *UICache) RemovePosition(sessionID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.positions, sessionID)
	atomic.AddUint64(&c.writes, 1)
}

// Clear removes all positions from the cache
func (c *UICache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.positions = make(map[string]monitor.Position)
	atomic.AddUint64(&c.writes, 1)
}

// GetStats returns cache statistics
func (c *UICache) GetStats() (positions, reads, writes uint64) {
	c.mu.RLock()
	positions = uint64(len(c.positions))
	c.mu.RUnlock()

	reads = atomic.LoadUint64(&c.reads)
	writes = atomic.LoadUint64(&c.writes)
	return positions, reads, writes
}

// CleanupStale removes positions older than the given duration
func (c *UICache) CleanupStale(maxAge time.Duration) int {
	c.mu.Lock()
	defer c.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	removed := 0

	for id, pos := range c.positions {
		if pos.UpdatedAt.Before(cutoff) {
			delete(c.positions, id)
			removed++
		}
	}

	if removed > 0 {
		c.logger.Info("Cleaned up stale positions",
			zap.Int("removed", removed),
			zap.Int("remaining", len(c.positions)))
	}

	return removed
}

// GlobalCache is the singleton UI cache instance
var GlobalCache *UICache

// InitCache initializes the global UI cache
func InitCache(logger *zap.Logger) {
	GlobalCache = NewUICache(logger)

	// Start periodic cleanup
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()

		for range ticker.C {
			GlobalCache.CleanupStale(30 * time.Minute)
		}
	}()
}
