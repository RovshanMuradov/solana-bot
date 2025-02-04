// internal/storage/storage.go
package storage

import (
	"context"

	"github.com/rovshanmuradov/solana-bot/internal/storage/models"
)

// Storage определяет интерфейс для работы с хранилищем
type Storage interface {
	// Транзакции
	SaveTransaction(ctx context.Context, tx *models.Transaction) error
	GetTransaction(ctx context.Context, signature string) (*models.Transaction, error)
	ListTransactions(ctx context.Context, walletAddress string, limit, offset int) ([]*models.Transaction, error)
	UpdateTransactionStatus(ctx context.Context, signature string, status string, errorMsg string) error

	// Задачи
	SaveTaskHistory(ctx context.Context, history *models.TaskHistory) error
	GetTaskStats(ctx context.Context, taskName string) (*models.TaskHistory, error)

	// Пулы
	SavePoolInfo(ctx context.Context, info *models.PoolInfo) error
	GetPoolInfo(ctx context.Context, poolID string) (*models.PoolInfo, error)

	// Миграции (без параметра migrationsPath)
	RunMigrations() error
}
