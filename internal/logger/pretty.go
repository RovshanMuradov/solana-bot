// internal/logger/pretty.go
package logger

import (
	"fmt"
	"os"
	"strings"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Colors for terminal output
const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
	ColorPurple = "\033[35m"
	ColorCyan   = "\033[36m"
	ColorWhite  = "\033[37m"
	ColorBold   = "\033[1m"
)

// PrettyEncoder creates a user-friendly console encoder
func PrettyEncoder() zapcore.Encoder {
	config := zapcore.EncoderConfig{
		MessageKey:     "msg",
		LevelKey:       "level",
		TimeKey:        "time",
		CallerKey:      "",
		StacktraceKey:  "",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    customLevelEncoder,
		EncodeTime:     customTimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   customCallerEncoder,
	}
	return zapcore.NewConsoleEncoder(config)
}

// customLevelEncoder formats log levels with colors
func customLevelEncoder(level zapcore.Level, enc zapcore.PrimitiveArrayEncoder) {
	switch level {
	case zapcore.DebugLevel:
		enc.AppendString(fmt.Sprintf("%s[DEBUG]%s", ColorCyan, ColorReset))
	case zapcore.InfoLevel:
		enc.AppendString(fmt.Sprintf("%s[INFO]%s", ColorGreen, ColorReset))
	case zapcore.WarnLevel:
		enc.AppendString(fmt.Sprintf("%s[WARN]%s", ColorYellow, ColorReset))
	case zapcore.ErrorLevel:
		enc.AppendString(fmt.Sprintf("%s[ERROR]%s", ColorRed, ColorReset))
	case zapcore.FatalLevel:
		enc.AppendString(fmt.Sprintf("%s[FATAL]%s", ColorRed+ColorBold, ColorReset))
	default:
		enc.AppendString(fmt.Sprintf("[%s]", level.CapitalString()))
	}
}

// customTimeEncoder formats time in a readable way
func customTimeEncoder(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(t.Format("15:04:05"))
}

// customCallerEncoder hides caller information for cleaner logs
func customCallerEncoder(caller zapcore.EntryCaller, enc zapcore.PrimitiveArrayEncoder) {
	// Don't show caller for cleaner output
}

// CreatePrettyLogger creates a logger with user-friendly output
func CreatePrettyLogger(debug bool) (*zap.Logger, error) {
	// Create a custom encoder that suppresses extra fields
	encoderConfig := zapcore.EncoderConfig{
		MessageKey:     "msg",
		LevelKey:       "level",
		TimeKey:        "time",
		CallerKey:      "",
		StacktraceKey:  "",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    customLevelEncoder,
		EncodeTime:     customTimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   customCallerEncoder,
	}

	// Custom core that filters out unwanted fields
	var core zapcore.Core
	if debug {
		core = zapcore.NewCore(
			zapcore.NewConsoleEncoder(encoderConfig),
			zapcore.AddSync(zapcore.Lock(os.Stdout)),
			zap.DebugLevel,
		)
	} else {
		core = zapcore.NewCore(
			zapcore.NewConsoleEncoder(encoderConfig),
			zapcore.AddSync(zapcore.Lock(os.Stdout)),
			zap.InfoLevel,
		)
	}

	// Create a custom core wrapper that filters out additional fields
	filteredCore := &FieldFilterCore{core: core}
	return zap.New(filteredCore), nil
}

// FormatMessage creates user-friendly log messages
func FormatMessage(msg string, fields ...zap.Field) string {
	// Extract common patterns and make them prettier
	switch {
	case strings.Contains(msg, "License validated"):
		return fmt.Sprintf("%s✓ License validated successfully%s", ColorGreen, ColorReset)

	case strings.Contains(msg, "Tasks loaded"):
		count := extractField(fields, "count")
		return fmt.Sprintf("%s📋 Loaded %s trading tasks%s", ColorBlue, count, ColorReset)

	case strings.Contains(msg, "Worker started"):
		return fmt.Sprintf("%s🚀 Trading worker started%s", ColorGreen, ColorReset)

	case strings.Contains(msg, "Executing task"):
		task := extractField(fields, "task")
		dex := extractField(fields, "DEX")
		token := extractField(fields, "token_mint")
		return fmt.Sprintf("%s⚡ Executing %s on %s%s\n    Token: %s", ColorCyan, task, dex, ColorReset, shortenAddress(token))

	case strings.Contains(msg, "Using Pump.fun"):
		return fmt.Sprintf("%s🎯 Smart DEX selected: Pump.fun (bonding curve active)%s", ColorPurple, ColorReset)

	case strings.Contains(msg, "Using Pump.swap"):
		return fmt.Sprintf("%s🎯 Smart DEX selected: Pump.swap (bonding curve completed)%s", ColorPurple, ColorReset)

	case strings.Contains(msg, "Transaction sent"):
		sig := extractField(fields, "signature")
		return fmt.Sprintf("%s📤 Transaction sent: %s%s", ColorYellow, shortenSignature(sig), ColorReset)

	case strings.Contains(msg, "Transaction confirmed"):
		sig := extractField(fields, "signature")
		return fmt.Sprintf("%s✅ Transaction confirmed: %s%s", ColorGreen, shortenSignature(sig), ColorReset)

	case strings.Contains(msg, "Operation completed successfully"):
		return fmt.Sprintf("%s🎉 Trade executed successfully!%s", ColorGreen+ColorBold, ColorReset)

	case strings.Contains(msg, "Token received"):
		balance := extractField(fields, "balance")
		return fmt.Sprintf("%s💰 Tokens received: %s%s", ColorGreen, balance, ColorReset)

	case strings.Contains(msg, "Tokens sold successfully"):
		return fmt.Sprintf("%s💸 Tokens sold successfully!%s", ColorGreen+ColorBold, ColorReset)

	case strings.Contains(msg, "Task channel closed"):
		return fmt.Sprintf("%s✓ All trading tasks completed%s", ColorGreen, ColorReset)

	default:
		return msg
	}
}

// Helper functions
func extractField(fields []zap.Field, key string) string {
	for _, field := range fields {
		if field.Key == key {
			return fmt.Sprintf("%v", field.Interface)
		}
	}
	return ""
}

func shortenAddress(addr string) string {
	if len(addr) > 8 {
		return addr[:4] + "..." + addr[len(addr)-4:]
	}
	return addr
}

func shortenSignature(sig string) string {
	if len(sig) > 16 {
		return sig[:8] + "..." + sig[len(sig)-8:]
	}
	return sig
}

// FieldFilterCore wraps a zapcore.Core to filter out unwanted fields
type FieldFilterCore struct {
	core zapcore.Core
}

func (c *FieldFilterCore) Enabled(level zapcore.Level) bool {
	return c.core.Enabled(level)
}

func (c *FieldFilterCore) With(fields []zapcore.Field) zapcore.Core {
	return &FieldFilterCore{core: c.core.With(fields)}
}

func (c *FieldFilterCore) Check(entry zapcore.Entry, checked *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	return c.core.Check(entry, checked)
}

func (c *FieldFilterCore) Write(entry zapcore.Entry, fields []zapcore.Field) error {
	// Filter out unwanted fields - only keep message
	var filteredFields []zapcore.Field

	// Create a cleaner message without extra data
	cleanMsg := entry.Message

	// Replace the entry message with clean version
	cleanEntry := entry
	cleanEntry.Message = cleanMsg

	return c.core.Write(cleanEntry, filteredFields)
}

func (c *FieldFilterCore) Sync() error {
	return c.core.Sync()
}

// CreatePrettyLoggerWithBuffer creates a logger with user-friendly output and log buffer
func CreatePrettyLoggerWithBuffer(debug bool, buffer *LogBuffer) (*zap.Logger, error) {
	// Create a custom encoder that suppresses extra fields
	encoderConfig := zapcore.EncoderConfig{
		MessageKey:     "msg",
		LevelKey:       "level",
		TimeKey:        "time",
		CallerKey:      "",
		StacktraceKey:  "",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    customLevelEncoder,
		EncodeTime:     customTimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   customCallerEncoder,
	}

	// Create base core for console output
	level := zap.InfoLevel
	if debug {
		level = zap.DebugLevel
	}

	// Create buffer core if buffer is provided
	var cores []zapcore.Core

	if buffer != nil {
		// Create JSON encoder for buffer (structured logs)
		jsonEncoder := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
		bufferCore := zapcore.NewCore(
			jsonEncoder,
			zapcore.AddSync(buffer),
			level,
		)
		cores = append(cores, bufferCore)
	} else {
		// Fallback to console output only if no buffer provided
		consoleCore := zapcore.NewCore(
			zapcore.NewConsoleEncoder(encoderConfig),
			zapcore.AddSync(zapcore.Lock(os.Stdout)),
			level,
		)
		cores = append(cores, &FieldFilterCore{core: consoleCore})
	}

	// Combine cores
	multiCore := zapcore.NewTee(cores...)
	return zap.New(multiCore), nil
}

// CreateTUILoggerWithBuffer creates a TUI-compatible logger that only writes to buffer
func CreateTUILoggerWithBuffer(debug bool, buffer *LogBuffer) (*zap.Logger, error) {
	if buffer == nil {
		return nil, fmt.Errorf("buffer is required for TUI logger")
	}

	level := zap.InfoLevel
	if debug {
		level = zap.DebugLevel
	}

	// Create clean encoder for buffer logs
	encoderConfig := zapcore.EncoderConfig{
		MessageKey:     "msg",
		LevelKey:       "level",
		TimeKey:        "time",
		CallerKey:      "",
		StacktraceKey:  "",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
	}

	// Only use buffer core - NO console output to avoid breaking TUI
	jsonEncoder := zapcore.NewJSONEncoder(encoderConfig)
	bufferCore := zapcore.NewCore(
		jsonEncoder,
		zapcore.AddSync(buffer),
		level,
	)

	return zap.New(bufferCore), nil
}
