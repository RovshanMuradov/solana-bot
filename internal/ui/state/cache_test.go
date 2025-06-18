package state

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/rovshanmuradov/solana-bot/internal/monitor"
	"go.uber.org/zap"
)

func TestUICacheConcurrentAccess(t *testing.T) {
	logger := zap.NewNop()
	cache := NewUICache(logger)

	var wg sync.WaitGroup
	numGoroutines := 10
	positionsPerGoroutine := 50

	// Concurrent writes
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < positionsPerGoroutine; j++ {
				pos := monitor.Position{
					SessionID:  fmt.Sprintf("session_%d_%d", id, j),
					WalletAddr: fmt.Sprintf("wallet_%d", id),
					TokenMint:  fmt.Sprintf("token_%d", j),
					Amount:     float64(j * 100),
					InitialSOL: 1.0,
					CurrentSOL: 1.5,
					PnL:        0.5,
					PnLPercent: 50.0,
					Status:     "active",
				}
				cache.SetPosition(pos)
			}
		}(i)
	}

	// Concurrent reads
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < positionsPerGoroutine; j++ {
				_ = cache.GetSnapshot()
				sessionID := fmt.Sprintf("session_%d_%d", id, j/2)
				_, _ = cache.GetPosition(sessionID)
			}
		}(i)
	}

	// Concurrent updates
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < positionsPerGoroutine; j++ {
				sessionID := fmt.Sprintf("session_%d_%d", id, j)
				update := monitor.PriceUpdate{
					Current: float64(j) * 2.0,
					Initial: 1.0,
					Percent: float64(j) * 100.0,
				}
				cache.UpdatePosition(sessionID, update)
			}
		}(i)
	}

	wg.Wait()

	positions, reads, writes := cache.GetStats()
	t.Logf("Positions: %d, Reads: %d, Writes: %d", positions, reads, writes)

	if positions == 0 {
		t.Error("Expected some positions in cache")
	}
	if reads == 0 {
		t.Error("Expected some read operations")
	}
	if writes == 0 {
		t.Error("Expected some write operations")
	}
}

func TestUICacheSnapshot(t *testing.T) {
	logger := zap.NewNop()
	cache := NewUICache(logger)

	// Add positions
	positions := []monitor.Position{
		{SessionID: "1", TokenMint: "token1", Status: "active"},
		{SessionID: "2", TokenMint: "token2", Status: "monitoring"},
		{SessionID: "3", TokenMint: "token3", Status: "closed"},
	}

	for _, pos := range positions {
		cache.SetPosition(pos)
	}

	// Get snapshot
	snapshot := cache.GetSnapshot()
	if len(snapshot) != 3 {
		t.Errorf("Expected 3 positions in snapshot, got %d", len(snapshot))
	}

	// Verify snapshot is a copy
	snapshot[0].TokenMint = "modified"

	// Original should be unchanged
	original, _ := cache.GetPosition("1")
	if original.TokenMint != "token1" {
		t.Error("Snapshot modification affected cache")
	}

	// Get active positions
	active := cache.GetActivePositions()
	if len(active) != 2 {
		t.Errorf("Expected 2 active positions, got %d", len(active))
	}
}

func TestUICacheCleanup(t *testing.T) {
	logger := zap.NewNop()
	cache := NewUICache(logger)

	// Add old position (manually set time to bypass SetPosition's UpdatedAt)
	cache.mu.Lock()
	cache.positions["old"] = monitor.Position{
		SessionID: "old",
		UpdatedAt: time.Now().Add(-1 * time.Hour),
	}
	cache.mu.Unlock()

	// Add recent position
	newPos := monitor.Position{
		SessionID: "new",
		UpdatedAt: time.Now(),
	}
	cache.SetPosition(newPos)

	// Cleanup old positions
	removed := cache.CleanupStale(30 * time.Minute)
	if removed != 1 {
		t.Errorf("Expected 1 position to be removed, got %d", removed)
	}

	// Verify only new position remains
	_, exists := cache.GetPosition("old")
	if exists {
		t.Error("Old position should have been removed")
	}

	_, exists = cache.GetPosition("new")
	if !exists {
		t.Error("New position should still exist")
	}
}

func TestGlobalCache(t *testing.T) {
	logger := zap.NewNop()
	InitCache(logger)

	// Test global cache operations
	pos := monitor.Position{
		SessionID: "global_test",
		TokenMint: "test_token",
		Status:    "active",
	}
	GlobalCache.SetPosition(pos)

	retrieved, exists := GlobalCache.GetPosition("global_test")
	if !exists {
		t.Error("Position should exist in global cache")
	}
	if retrieved.TokenMint != "test_token" {
		t.Errorf("Expected token mint 'test_token', got '%s'", retrieved.TokenMint)
	}
}
