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
	gormlogger "gorm.io/gorm/logger"
)

// gormLogger реализует интерфейс logger.Interface для GORM с интеграцией zap.
type gormLogger struct {
	zapLogger *zap.Logger
	logLevel  gormlogger.LogLevel
}

func newGormLogger(zapLogger *zap.Logger) gormlogger.Interface {
	return &gormLogger{
		zapLogger: zapLogger,
		logLevel:  gormlogger.Info,
	}
}

func (l *gormLogger) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	newLogger := *l
	newLogger.logLevel = level
	return &newLogger
}

func (l *gormLogger) Info(ctx context.Context, msg string, data ...interface{}) {
	if l.logLevel >= gormlogger.Info {
		l.zapLogger.Sugar().Infof(msg, data...)
	}
}

func (l *gormLogger) Warn(ctx context.Context, msg string, data ...interface{}) {
	if l.logLevel >= gormlogger.Warn {
		l.zapLogger.Sugar().Warnf(msg, data...)
	}
}

func (l *gormLogger) Error(ctx context.Context, msg string, data ...interface{}) {
	if l.logLevel >= gormlogger.Error {
		l.zapLogger.Sugar().Errorf(msg, data...)
	}
}

func (l *gormLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	if l.logLevel <= gormlogger.Silent {
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
	if l.logLevel >= gormlogger.Info {
		l.zapLogger.Info("trace", fields...)
	}
}

type postgresStorage struct {
	db     *gorm.DB
	logger *zap.Logger
}

func NewStorage(dsn string, zapLogger *zap.Logger) (storage.Storage, error) {
	gl := newGormLogger(zapLogger.Named("gorm"))
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gl,
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

// RunMigrations использует GORM AutoMigrate без параметра migrationsPath.
func (p *postgresStorage) RunMigrations() error {
	// Получаем advisory lock
	var lockObtained bool
	err := p.db.Raw("SELECT pg_try_advisory_lock(101)").Scan(&lockObtained).Error
	if err != nil {
		return fmt.Errorf("failed to acquire migration lock: %w", err)
	}
	if !lockObtained {
		return fmt.Errorf("another migration is in progress")
	}
	defer p.db.Exec("SELECT pg_advisory_unlock(101)")
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
