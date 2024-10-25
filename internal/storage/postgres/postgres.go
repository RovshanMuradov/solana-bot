// internal/storage/postgres/postgres.go
package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/rovshanmuradov/solana-bot/internal/storage"
	"github.com/rovshanmuradov/solana-bot/internal/storage/models"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// gormLogger реализует интерфейс logger.Interface для GORM
type gormLogger struct {
	zapLogger *zap.Logger
	logLevel  logger.LogLevel
}

// newGormLogger создает новый логгер для GORM
func newGormLogger(zapLogger *zap.Logger) logger.Interface {
	return &gormLogger{
		zapLogger: zapLogger,
		logLevel:  logger.Info,
	}
}

// LogMode реализация интерфейса logger.Interface
func (l *gormLogger) LogMode(level logger.LogLevel) logger.Interface {
	newLogger := *l
	newLogger.logLevel = level
	return &newLogger
}

// Info реализация интерфейса logger.Interface
func (l *gormLogger) Info(_ context.Context, msg string, data ...interface{}) {
	if l.logLevel >= logger.Info {
		l.zapLogger.Sugar().Infof(msg, data...)
	}
}

// Warn реализация интерфейса logger.Interface
func (l *gormLogger) Warn(_ context.Context, msg string, data ...interface{}) {
	if l.logLevel >= logger.Warn {
		l.zapLogger.Sugar().Warnf(msg, data...)
	}
}

// Error реализация интерфейса logger.Interface
func (l *gormLogger) Error(_ context.Context, msg string, data ...interface{}) {
	if l.logLevel >= logger.Error {
		l.zapLogger.Sugar().Errorf(msg, data...)
	}
}

// Trace реализация интерфейса logger.Interface
func (l *gormLogger) Trace(_ context.Context, begin time.Time, fc func() (string, int64), err error) {
	if l.logLevel <= logger.Silent {
		return
	}

	elapsed := time.Since(begin)
	sql, rows := fc()

	fields := []zap.Field{
		zap.Duration("elapsed", elapsed),
		zap.String("sql", sql),
		zap.Int64("rows", rows),
	}

	if err != nil {
		l.zapLogger.Error("trace", append(fields, zap.Error(err))...)
		return
	}

	if l.logLevel >= logger.Info {
		l.zapLogger.Info("trace", fields...)
	}
}

// postgresStorage реализует интерфейс Storage
type postgresStorage struct {
	db     *gorm.DB
	logger *zap.Logger
}

func NewStorage(dsn string, zapLogger *zap.Logger) (storage.Storage, error) {
	gormLogger := newGormLogger(zapLogger.Named("gorm"))

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormLogger,
		NowFunc: func() time.Time {
			return time.Now().UTC()
		},
		DisableForeignKeyConstraintWhenMigrating: true,
		SkipDefaultTransaction:                   true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get database instance: %w", err)
	}

	// Настройка пула соединений
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)

	return &postgresStorage{
		db:     db,
		logger: zapLogger,
	}, nil
}

// RunMigrations теперь использует GORM AutoMigrate
func (p *postgresStorage) RunMigrations(_ string) error {
	// Сначала попробуем получить блокировку
	var lockObtained bool
	err := p.db.Raw("SELECT pg_try_advisory_lock(101)").Scan(&lockObtained).Error
	if err != nil {
		return fmt.Errorf("failed to acquire migration lock: %w", err)
	}
	if !lockObtained {
		return fmt.Errorf("another migration is in progress")
	}
	defer p.db.Exec("SELECT pg_advisory_unlock(101)")

	// Используем GORM AutoMigrate
	err = p.db.AutoMigrate(
		&models.Transaction{},
		&models.TaskHistory{},
		&models.PoolInfo{},
	)
	if err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	return nil
}

// Реализация методов интерфейса Storage
func (p *postgresStorage) SaveTransaction(ctx context.Context, tx *models.Transaction) error {
	return p.db.WithContext(ctx).Create(tx).Error
}

func (p *postgresStorage) GetTransaction(ctx context.Context, signature string) (*models.Transaction, error) {
	var tx models.Transaction
	err := p.db.WithContext(ctx).Where("signature = ?", signature).First(&tx).Error
	if err != nil {
		return nil, err
	}
	return &tx, nil
}

func (p *postgresStorage) ListTransactions(ctx context.Context, walletAddress string, limit, offset int) ([]*models.Transaction, error) {
	var txs []*models.Transaction
	err := p.db.WithContext(ctx).
		Where("wallet_address = ?", walletAddress).
		Order("created_at desc").
		Limit(limit).
		Offset(offset).
		Find(&txs).Error
	return txs, err
}

func (p *postgresStorage) UpdateTransactionStatus(ctx context.Context, signature string, status string, errorMsg string) error {
	return p.db.WithContext(ctx).Model(&models.Transaction{}).
		Where("signature = ?", signature).
		Updates(map[string]interface{}{
			"status":        status,
			"error_message": errorMsg,
		}).Error
}

func (p *postgresStorage) SaveTaskHistory(ctx context.Context, history *models.TaskHistory) error {
	return p.db.WithContext(ctx).Create(history).Error
}

func (p *postgresStorage) GetTaskStats(ctx context.Context, taskName string) (*models.TaskHistory, error) {
	var history models.TaskHistory
	err := p.db.WithContext(ctx).Where("task_name = ?", taskName).First(&history).Error
	if err != nil {
		return nil, err
	}
	return &history, nil
}

func (p *postgresStorage) SavePoolInfo(ctx context.Context, info *models.PoolInfo) error {
	return p.db.WithContext(ctx).Create(info).Error
}

func (p *postgresStorage) GetPoolInfo(ctx context.Context, poolID string) (*models.PoolInfo, error) {
	var info models.PoolInfo
	err := p.db.WithContext(ctx).Where("pool_id = ?", poolID).First(&info).Error
	if err != nil {
		return nil, err
	}
	return &info, nil
}
