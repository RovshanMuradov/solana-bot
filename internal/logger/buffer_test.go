package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestLogBufferConcurrentAccess(t *testing.T) {
	tempDir := t.TempDir()
	spillFile := filepath.Join(tempDir, "test_spill.log")
	logger := zap.NewNop()

	buffer, err := NewLogBuffer(100, spillFile, logger)
	if err != nil {
		t.Fatalf("Failed to create log buffer: %v", err)
	}
	defer buffer.Close()

	// Start periodic flush
	done := buffer.StartPeriodicFlush(50 * time.Millisecond)
	defer close(done)

	// Simulate concurrent log writes
	var wg sync.WaitGroup
	numGoroutines := 10
	logsPerGoroutine := 100

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < logsPerGoroutine; j++ {
				fields := map[string]interface{}{
					"goroutine": id,
					"iteration": j,
				}
				err := buffer.Add("INFO", fmt.Sprintf("Log from goroutine %d, iteration %d", id, j), fields)
				if err != nil {
					t.Errorf("Failed to add log: %v", err)
				}
			}
		}(i)
	}

	// Concurrent reads
	go func() {
		for i := 0; i < 50; i++ {
			logs := buffer.GetRecentLogs(10)
			_ = logs
			time.Sleep(10 * time.Millisecond)
		}
	}()

	// Concurrent stats
	go func() {
		for i := 0; i < 50; i++ {
			total, spilled := buffer.GetStats()
			_ = total
			_ = spilled
			time.Sleep(10 * time.Millisecond)
		}
	}()

	wg.Wait()

	// Final flush
	if err := buffer.Flush(); err != nil {
		t.Errorf("Failed to flush: %v", err)
	}

	total, spilled := buffer.GetStats()
	t.Logf("Total entries: %d, Spilled entries: %d", total, spilled)

	// Should have processed all logs
	expectedTotal := uint64(numGoroutines * logsPerGoroutine)
	if total != expectedTotal {
		t.Errorf("Expected %d total entries, got %d", expectedTotal, total)
	}

	// Should have spilled some entries (buffer size is 100)
	if spilled == 0 && total > 100 {
		t.Error("Expected some entries to be spilled")
	}

	// Check spill file exists
	if _, err := os.Stat(spillFile); os.IsNotExist(err) {
		t.Error("Spill file should exist")
	}
}

func TestLogBufferRingBufferBehavior(t *testing.T) {
	tempDir := t.TempDir()
	spillFile := filepath.Join(tempDir, "test_ring.log")
	logger := zap.NewNop()

	bufferSize := 5
	buffer, err := NewLogBuffer(bufferSize, spillFile, logger)
	if err != nil {
		t.Fatalf("Failed to create log buffer: %v", err)
	}
	defer buffer.Close()

	// Add more logs than buffer size
	for i := 0; i < 10; i++ {
		err := buffer.Add("INFO", fmt.Sprintf("Log %d", i), nil)
		if err != nil {
			t.Errorf("Failed to add log: %v", err)
		}
	}

	// Get recent logs
	logs := buffer.GetRecentLogs(10)
	t.Logf("Buffer size: %d, Retrieved logs: %d", bufferSize, len(logs))

	// Should only have buffer size worth of logs in memory
	if len(logs) != bufferSize {
		t.Errorf("Expected %d logs in buffer, got %d", bufferSize, len(logs))
	}

	// Check that we have the most recent logs
	lastLog := logs[len(logs)-1]
	if lastLog.Message != "Log 9" {
		t.Errorf("Expected last log to be 'Log 9', got '%s'", lastLog.Message)
	}
}
