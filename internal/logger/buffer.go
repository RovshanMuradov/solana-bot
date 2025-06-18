package logger

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go.uber.org/zap"
)

// LogEntry represents a single log entry in the buffer
type LogEntry struct {
	Timestamp time.Time              `json:"timestamp"`
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
}

// LogBuffer provides a thread-safe ring buffer for logs with file backup
type LogBuffer struct {
	mu           sync.Mutex
	ringBuffer   []LogEntry
	maxSize      int
	currentIndex int
	wrapped      bool
	spillFile    *os.File
	spillWriter  *bufio.Writer
	logger       *zap.Logger

	// Stats
	totalEntries   uint64
	spilledEntries uint64
}

// NewLogBuffer creates a new log buffer with the specified size
func NewLogBuffer(maxSize int, spillFilePath string, logger *zap.Logger) (*LogBuffer, error) {
	// Ensure directory exists
	dir := filepath.Dir(spillFilePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	// Open spill file in append mode
	spillFile, err := os.OpenFile(spillFilePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open spill file: %w", err)
	}

	return &LogBuffer{
		ringBuffer:  make([]LogEntry, maxSize),
		maxSize:     maxSize,
		spillFile:   spillFile,
		spillWriter: bufio.NewWriter(spillFile),
		logger:      logger,
	}, nil
}

// Add adds a new log entry to the buffer
func (lb *LogBuffer) Add(level, message string, fields map[string]interface{}) error {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     level,
		Message:   message,
		Fields:    fields,
	}

	// Add to ring buffer
	lb.ringBuffer[lb.currentIndex] = entry
	lb.currentIndex = (lb.currentIndex + 1) % lb.maxSize

	if lb.currentIndex == 0 && lb.totalEntries > 0 {
		lb.wrapped = true
	}

	lb.totalEntries++

	// If buffer is full, spill oldest entry to file
	if lb.wrapped {
		oldestIndex := lb.currentIndex
		oldestEntry := lb.ringBuffer[oldestIndex]

		if err := lb.spillToFile(oldestEntry); err != nil {
			lb.logger.Error("Failed to spill log entry to file", zap.Error(err))
			return err
		}
		lb.spilledEntries++
	}

	return nil
}

// spillToFile writes an entry to the spill file
func (lb *LogBuffer) spillToFile(entry LogEntry) error {
	// Write as JSON for easy parsing
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal log entry: %w", err)
	}

	if _, err := lb.spillWriter.Write(data); err != nil {
		return fmt.Errorf("failed to write to spill file: %w", err)
	}

	if _, err := lb.spillWriter.WriteString("\n"); err != nil {
		return fmt.Errorf("failed to write newline: %w", err)
	}

	// Don't flush on every write for performance, rely on periodic flush
	return nil
}

// GetRecentLogs returns the most recent log entries (up to limit)
func (lb *LogBuffer) GetRecentLogs(limit int) []LogEntry {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	var logs []LogEntry
	count := lb.maxSize
	if !lb.wrapped {
		count = lb.currentIndex
	}

	if limit > 0 && limit < count {
		count = limit
	}

	// Start from the oldest entry
	startIndex := lb.currentIndex
	if lb.wrapped {
		// If wrapped, oldest is at currentIndex
		startIndex = lb.currentIndex
	} else {
		// If not wrapped, start from 0
		startIndex = 0
	}

	for i := 0; i < count; i++ {
		index := (startIndex + i) % lb.maxSize
		if lb.wrapped || index < lb.currentIndex {
			logs = append(logs, lb.ringBuffer[index])
		}
	}

	return logs
}

// Flush forces a write of any buffered data to the spill file
func (lb *LogBuffer) Flush() error {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	if err := lb.spillWriter.Flush(); err != nil {
		return fmt.Errorf("failed to flush spill writer: %w", err)
	}

	if err := lb.spillFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync spill file: %w", err)
	}

	return nil
}

// Close closes the log buffer and ensures all data is written
func (lb *LogBuffer) Close() error {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	// Write all remaining entries to spill file
	if lb.wrapped {
		// Write all entries if buffer wrapped
		for i := 0; i < lb.maxSize; i++ {
			index := (lb.currentIndex + i) % lb.maxSize
			if err := lb.spillToFile(lb.ringBuffer[index]); err != nil {
				lb.logger.Error("Failed to spill entry during close", zap.Error(err))
			}
		}
	} else {
		// Write only valid entries if not wrapped
		for i := 0; i < lb.currentIndex; i++ {
			if err := lb.spillToFile(lb.ringBuffer[i]); err != nil {
				lb.logger.Error("Failed to spill entry during close", zap.Error(err))
			}
		}
	}

	// Flush and close
	if err := lb.spillWriter.Flush(); err != nil {
		return fmt.Errorf("failed to flush during close: %w", err)
	}

	if err := lb.spillFile.Close(); err != nil {
		return fmt.Errorf("failed to close spill file: %w", err)
	}

	lb.logger.Info("Log buffer closed",
		zap.Uint64("totalEntries", lb.totalEntries),
		zap.Uint64("spilledEntries", lb.spilledEntries))

	return nil
}

// GetStats returns buffer statistics
func (lb *LogBuffer) GetStats() (total, spilled uint64) {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	return lb.totalEntries, lb.spilledEntries
}

// StartPeriodicFlush starts a goroutine that periodically flushes the buffer
func (lb *LogBuffer) StartPeriodicFlush(interval time.Duration) chan struct{} {
	done := make(chan struct{})

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := lb.Flush(); err != nil {
					lb.logger.Error("Periodic flush failed", zap.Error(err))
				}
			case <-done:
				return
			}
		}
	}()

	return done
}
