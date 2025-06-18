package logger

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go.uber.org/zap"
)

// SafeFileWriter provides thread-safe file writing with buffering and periodic flush
type SafeFileWriter struct {
	mu       sync.Mutex
	writer   *bufio.Writer
	file     *os.File
	ticker   *time.Ticker
	done     chan struct{}
	logger   *zap.Logger
	filePath string

	// Stats
	writtenLines uint64
	flushCount   uint64
}

// NewSafeFileWriter creates a new thread-safe file writer
func NewSafeFileWriter(filePath string, flushInterval time.Duration, logger *zap.Logger) (*SafeFileWriter, error) {
	// Ensure directory exists
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	// Open file in append mode
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}

	sfw := &SafeFileWriter{
		writer:   bufio.NewWriter(file),
		file:     file,
		ticker:   time.NewTicker(flushInterval),
		done:     make(chan struct{}),
		logger:   logger,
		filePath: filePath,
	}

	// Start periodic flush goroutine
	go sfw.periodicFlush()

	return sfw, nil
}

// Write writes data to the file in a thread-safe manner
func (sfw *SafeFileWriter) Write(data []byte) (int, error) {
	sfw.mu.Lock()
	defer sfw.mu.Unlock()

	n, err := sfw.writer.Write(data)
	if err != nil {
		return n, fmt.Errorf("failed to write data: %w", err)
	}

	sfw.writtenLines++
	return n, nil
}

// WriteLine writes a line to the file with a newline appended
func (sfw *SafeFileWriter) WriteLine(line string) error {
	sfw.mu.Lock()
	defer sfw.mu.Unlock()

	if _, err := sfw.writer.WriteString(line); err != nil {
		return fmt.Errorf("failed to write line: %w", err)
	}

	if _, err := sfw.writer.WriteString("\n"); err != nil {
		return fmt.Errorf("failed to write newline: %w", err)
	}

	sfw.writtenLines++
	return nil
}

// Flush forces a write of any buffered data
func (sfw *SafeFileWriter) Flush() error {
	sfw.mu.Lock()
	defer sfw.mu.Unlock()

	if err := sfw.writer.Flush(); err != nil {
		return fmt.Errorf("failed to flush buffer: %w", err)
	}

	if err := sfw.file.Sync(); err != nil {
		return fmt.Errorf("failed to sync file: %w", err)
	}

	sfw.flushCount++
	return nil
}

// periodicFlush runs in a goroutine to periodically flush the buffer
func (sfw *SafeFileWriter) periodicFlush() {
	for {
		select {
		case <-sfw.ticker.C:
			if err := sfw.Flush(); err != nil {
				sfw.logger.Error("Periodic flush failed",
					zap.String("file", sfw.filePath),
					zap.Error(err))
			}
		case <-sfw.done:
			return
		}
	}
}

// Close closes the writer and ensures all data is written
func (sfw *SafeFileWriter) Close() error {
	// Stop periodic flush
	close(sfw.done)
	sfw.ticker.Stop()

	sfw.mu.Lock()
	defer sfw.mu.Unlock()

	// Final flush
	if err := sfw.writer.Flush(); err != nil {
		return fmt.Errorf("failed to flush on close: %w", err)
	}

	if err := sfw.file.Close(); err != nil {
		return fmt.Errorf("failed to close file: %w", err)
	}

	sfw.logger.Info("Safe file writer closed",
		zap.String("file", sfw.filePath),
		zap.Uint64("writtenLines", sfw.writtenLines),
		zap.Uint64("flushCount", sfw.flushCount))

	return nil
}

// GetStats returns writer statistics
func (sfw *SafeFileWriter) GetStats() (lines, flushes uint64) {
	sfw.mu.Lock()
	defer sfw.mu.Unlock()
	return sfw.writtenLines, sfw.flushCount
}

// SafeCSVWriter provides thread-safe CSV writing
type SafeCSVWriter struct {
	mu       sync.Mutex
	writer   *csv.Writer
	file     *os.File
	ticker   *time.Ticker
	done     chan struct{}
	logger   *zap.Logger
	filePath string

	// Stats
	writtenRecords uint64
	flushCount     uint64
}

// NewSafeCSVWriter creates a new thread-safe CSV writer
func NewSafeCSVWriter(filePath string, flushInterval time.Duration, logger *zap.Logger) (*SafeCSVWriter, error) {
	// Ensure directory exists
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	// Open file in append mode
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}

	// Check if file is empty to write header
	stat, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	csvWriter := csv.NewWriter(file)

	scw := &SafeCSVWriter{
		writer:   csvWriter,
		file:     file,
		ticker:   time.NewTicker(flushInterval),
		done:     make(chan struct{}),
		logger:   logger,
		filePath: filePath,
	}

	// Write header if file is empty
	if stat.Size() == 0 {
		header := []string{"timestamp", "wallet", "token", "action", "amount", "price", "pnl", "signature"}
		// Write header directly without using WriteRecord to avoid counting it
		scw.mu.Lock()
		if err := scw.writer.Write(header); err != nil {
			scw.mu.Unlock()
			file.Close()
			return nil, fmt.Errorf("failed to write header: %w", err)
		}
		scw.writer.Flush()
		scw.mu.Unlock()
	}

	// Start periodic flush goroutine
	go scw.periodicFlush()

	return scw, nil
}

// WriteRecord writes a CSV record in a thread-safe manner
func (scw *SafeCSVWriter) WriteRecord(record []string) error {
	scw.mu.Lock()
	defer scw.mu.Unlock()

	if err := scw.writer.Write(record); err != nil {
		return fmt.Errorf("failed to write record: %w", err)
	}

	scw.writtenRecords++
	return nil
}

// Flush forces a write of any buffered data
func (scw *SafeCSVWriter) Flush() error {
	scw.mu.Lock()
	defer scw.mu.Unlock()

	scw.writer.Flush()
	if err := scw.writer.Error(); err != nil {
		return fmt.Errorf("CSV writer error: %w", err)
	}

	if err := scw.file.Sync(); err != nil {
		return fmt.Errorf("failed to sync file: %w", err)
	}

	scw.flushCount++
	return nil
}

// periodicFlush runs in a goroutine to periodically flush the buffer
func (scw *SafeCSVWriter) periodicFlush() {
	for {
		select {
		case <-scw.ticker.C:
			if err := scw.Flush(); err != nil {
				scw.logger.Error("Periodic CSV flush failed",
					zap.String("file", scw.filePath),
					zap.Error(err))
			}
		case <-scw.done:
			return
		}
	}
}

// Close closes the CSV writer and ensures all data is written
func (scw *SafeCSVWriter) Close() error {
	// Stop periodic flush
	close(scw.done)
	scw.ticker.Stop()

	scw.mu.Lock()
	defer scw.mu.Unlock()

	// Final flush
	scw.writer.Flush()
	if err := scw.writer.Error(); err != nil {
		return fmt.Errorf("CSV writer error on close: %w", err)
	}

	if err := scw.file.Close(); err != nil {
		return fmt.Errorf("failed to close file: %w", err)
	}

	scw.logger.Info("Safe CSV writer closed",
		zap.String("file", scw.filePath),
		zap.Uint64("writtenRecords", scw.writtenRecords),
		zap.Uint64("flushCount", scw.flushCount))

	return nil
}

// GetStats returns CSV writer statistics
func (scw *SafeCSVWriter) GetStats() (records, flushes uint64) {
	scw.mu.Lock()
	defer scw.mu.Unlock()
	return scw.writtenRecords, scw.flushCount
}
