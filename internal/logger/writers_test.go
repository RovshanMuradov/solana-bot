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

func TestSafeFileWriterConcurrentWrites(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test_safe_writer.log")
	logger := zap.NewNop()

	writer, err := NewSafeFileWriter(testFile, 50*time.Millisecond, logger)
	if err != nil {
		t.Fatalf("Failed to create safe file writer: %v", err)
	}
	defer writer.Close()

	// Concurrent writes
	var wg sync.WaitGroup
	numGoroutines := 10
	linesPerGoroutine := 100

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < linesPerGoroutine; j++ {
				line := fmt.Sprintf("Goroutine %d, Line %d", id, j)
				if err := writer.WriteLine(line); err != nil {
					t.Errorf("Failed to write line: %v", err)
				}
			}
		}(i)
	}

	// Concurrent flushes
	flushDone := make(chan struct{})
	go func() {
		defer close(flushDone)
		for i := 0; i < 20; i++ {
			if err := writer.Flush(); err != nil {
				// Don't use t.Errorf in goroutine after test might complete
				logger.Error("Failed to flush", zap.Error(err))
			}
			time.Sleep(25 * time.Millisecond)
		}
	}()

	// Concurrent stats reading
	statsDone := make(chan struct{})
	go func() {
		defer close(statsDone)
		for i := 0; i < 50; i++ {
			lines, flushes := writer.GetStats()
			_ = lines
			_ = flushes
			time.Sleep(10 * time.Millisecond)
		}
	}()

	wg.Wait()

	// Wait for background goroutines
	select {
	case <-flushDone:
		// Flush goroutine completed
	case <-time.After(2 * time.Second):
		t.Error("Flush goroutine timeout")
	}

	select {
	case <-statsDone:
		// Stats goroutine completed
	case <-time.After(2 * time.Second):
		t.Error("Stats goroutine timeout")
	}

	// Final flush
	if err := writer.Flush(); err != nil {
		t.Errorf("Failed final flush: %v", err)
	}

	lines, flushes := writer.GetStats()
	t.Logf("Written lines: %d, Flush count: %d", lines, flushes)

	expectedLines := uint64(numGoroutines * linesPerGoroutine)
	if lines != expectedLines {
		t.Errorf("Expected %d lines, got %d", expectedLines, lines)
	}

	// File should exist and have content
	info, err := os.Stat(testFile)
	if err != nil {
		t.Errorf("Failed to stat file: %v", err)
	}
	if info.Size() == 0 {
		t.Error("File should not be empty")
	}
}

func TestSafeCSVWriterConcurrentWrites(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test_safe_csv.csv")
	logger := zap.NewNop()

	writer, err := NewSafeCSVWriter(testFile, 50*time.Millisecond, logger)
	if err != nil {
		t.Fatalf("Failed to create safe CSV writer: %v", err)
	}
	defer writer.Close()

	// Concurrent writes
	var wg sync.WaitGroup
	numGoroutines := 5
	recordsPerGoroutine := 50

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < recordsPerGoroutine; j++ {
				record := []string{
					time.Now().Format(time.RFC3339),
					fmt.Sprintf("wallet_%d", id),
					fmt.Sprintf("token_%d", j),
					"buy",
					"0.001",
					"100.5",
					"10.5",
					fmt.Sprintf("sig_%d_%d", id, j),
				}
				if err := writer.WriteRecord(record); err != nil {
					t.Errorf("Failed to write record: %v", err)
				}
			}
		}(i)
	}

	// Concurrent flushes
	csvFlushDone := make(chan struct{})
	go func() {
		defer close(csvFlushDone)
		for i := 0; i < 10; i++ {
			if err := writer.Flush(); err != nil {
				// Don't use t.Errorf in goroutine after test might complete
				logger.Error("CSV flush failed", zap.Error(err))
			}
			time.Sleep(50 * time.Millisecond)
		}
	}()

	wg.Wait()

	// Wait for flush goroutine
	select {
	case <-csvFlushDone:
		// Flush goroutine completed
	case <-time.After(2 * time.Second):
		t.Error("CSV flush goroutine timeout")
	}

	// Final flush
	if err := writer.Flush(); err != nil {
		t.Errorf("Failed final flush: %v", err)
	}

	records, flushes := writer.GetStats()
	t.Logf("Written records: %d, Flush count: %d", records, flushes)

	// Expected records = data records (not counting the header which was written in constructor)
	expectedRecords := uint64(numGoroutines * recordsPerGoroutine)
	if records != expectedRecords {
		t.Errorf("Expected %d records (excluding header), got %d", expectedRecords, records)
	}

	// File should exist and have content
	info, err := os.Stat(testFile)
	if err != nil {
		t.Errorf("Failed to stat file: %v", err)
	}
	if info.Size() == 0 {
		t.Error("CSV file should not be empty")
	}
}

func TestSafeFileWriterWithSlowWrites(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test_slow_writes.log")
	logger := zap.NewNop()

	// Very short flush interval
	writer, err := NewSafeFileWriter(testFile, 10*time.Millisecond, logger)
	if err != nil {
		t.Fatalf("Failed to create safe file writer: %v", err)
	}
	defer writer.Close()

	// Write slowly to test periodic flush
	for i := 0; i < 10; i++ {
		line := fmt.Sprintf("Slow write %d", i)
		if err := writer.WriteLine(line); err != nil {
			t.Errorf("Failed to write line: %v", err)
		}
		time.Sleep(15 * time.Millisecond) // Longer than flush interval
	}

	lines, flushes := writer.GetStats()
	t.Logf("Lines: %d, Flushes: %d", lines, flushes)

	// Should have multiple flushes due to periodic flush
	if flushes < 2 {
		t.Error("Expected multiple periodic flushes")
	}
}
