// internal/utils/logger/logger.go
package logger

import (
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/rovshanmuradov/solana-bot/internal/types"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Logger расширяет функционал zap.Logger
type Logger struct {
	*zap.Logger
	config *Config
}

// Для удобства добавим константы статусов трейда
const (
	TradePending   = "pending"
	TradeCompleted = "completed"
	TradeFailed    = "failed"
	TradeCancelled = "cancelled"
)

// New создает новый логгер с расширенной функциональностью
func New(cfg *Config) (*Logger, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	// Настройка ротации логов
	logRotator := &lumberjack.Logger{
		Filename:   cfg.LogFile,
		MaxSize:    cfg.MaxSize,
		MaxBackups: cfg.MaxBackups,
		MaxAge:     cfg.MaxAge,
		Compress:   cfg.Compress,
	}

	// Базовая конфигурация энкодера
	encoderConfig := zap.NewProductionEncoderConfig()
	if cfg.Development {
		encoderConfig = zap.NewDevelopmentEncoderConfig()
	}

	// Улучшаем конфигурацию энкодера
	encoderConfig.TimeKey = "timestamp"
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder
	encoderConfig.EncodeDuration = zapcore.StringDurationEncoder
	encoderConfig.EncodeCaller = zapcore.ShortCallerEncoder

	// Создаем энкодеры для консоли и файла
	consoleEncoder := zapcore.NewConsoleEncoder(encoderConfig)
	fileEncoder := zapcore.NewJSONEncoder(encoderConfig)

	// Определяем уровень логирования
	var level zapcore.Level
	if cfg.Development {
		level = zapcore.DebugLevel
	} else {
		level = zapcore.InfoLevel
	}

	// Создаем core с поддержкой нескольких writers
	core := zapcore.NewTee(
		zapcore.NewCore(consoleEncoder, zapcore.AddSync(os.Stdout), level),
		zapcore.NewCore(fileEncoder, zapcore.AddSync(logRotator), level),
	)

	// Создаем логгер с дополнительными опциями
	logger := &Logger{
		Logger: zap.New(core,
			zap.AddCaller(),
			zap.AddStacktrace(zapcore.ErrorLevel),
			zap.AddCallerSkip(1),
		),
		config: cfg,
	}

	return logger, nil
}

// WithTransaction добавляет контекст транзакции к логам
func (l *Logger) WithTransaction(txHash string) *zap.Logger {
	return l.With(
		zap.String("tx_hash", txHash),
		zap.Time("tx_time", time.Now().UTC()),
	)
}

// WithOperation создает логгер для конкретной операции
func (l *Logger) WithOperation(operation string) *zap.Logger {
	return l.With(
		zap.String("operation", operation),
		zap.String("correlation_id", uuid.New().String()),
		zap.Time("start_time", time.Now().UTC()),
	)
}

// WithComponent добавляет информацию о компоненте системы
func (l *Logger) WithComponent(component string) *zap.Logger {
	return l.With(zap.String("component", component))
}

// WithUser добавляет информацию о пользователе/wallet
func (l *Logger) WithUser(userID string) *zap.Logger {
	return l.With(zap.String("user_id", userID))
}

// LogError логирует ошибку с дополнительным контекстом
func (l *Logger) LogError(msg string, err error, fields ...zap.Field) {
	if err != nil {
		fields = append(fields, zap.Error(err))
	}
	l.Error(msg, fields...)
}

// Sync реализует безопасный вызов Sync
func (l *Logger) Sync() error {
	err := l.Logger.Sync()
	if err != nil && (err.Error() == "sync /dev/stdout: invalid argument" || 
                     err.Error() == "sync /dev/stderr: inappropriate ioctl for device") {
		return nil
	}
	return err
}

// WithTask логирует информацию о задаче
func (l *Logger) WithTask(task *types.Task) *zap.Logger {
	return l.With(
		zap.String("task_name", task.TaskName),
		zap.String("dex", task.DEXName),
		zap.String("source_token", task.SourceToken),
		zap.String("target_token", task.TargetToken),
		zap.Float64("amount_in", task.AmountIn),
		zap.String("slippage_type", string(task.SlippageConfig.Type)),
		zap.Float64("slippage_value", task.SlippageConfig.Value),
		zap.Float64("priority_fee", task.PriorityFee),
	)
}

// // WithPool логирует информацию о пуле
// func (l *Logger) WithPool(pool *raydium.Pool) *zap.Logger {
// 	return l.With(
// 		zap.String("pool_id", pool.AmmID),
// 		zap.String("program_id", pool.AmmProgramID),
// 		zap.String("pool_token_a", pool.PoolCoinTokenAccount),
// 		zap.String("pool_token_b", pool.PoolPcTokenAccount),
// 	)
// }

// TrackPerformance отслеживает производительность операции
func (l *Logger) TrackPerformance(operation string) (end func()) {
	start := time.Now()
	opLogger := l.WithOperation(operation)

	opLogger.Debug("Starting operation")

	return func() {
		duration := time.Since(start)
		opLogger.Debug("Operation completed",
			zap.Duration("duration", duration),
			zap.Float64("duration_ms", float64(duration.Microseconds())/1000),
		)
	}
}
