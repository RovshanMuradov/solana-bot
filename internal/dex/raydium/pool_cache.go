// internal/dex/raydium/pool_cache.go
package raydium

import (
	"fmt"
	"sync"
	"time"

	"github.com/gagliardetto/solana-go"
	"go.uber.org/zap"
)

const (
	defaultCacheTTL = 15 * time.Minute
	minCacheTTL     = time.Minute
	maxCacheTTL     = time.Hour
	cleanupInterval = 5 * time.Minute
)

// CacheEntry представляет запись в кэше с временем жизни и метаданными
type CacheEntry struct {
	Pool           *Pool
	ExpireAt       time.Time
	LastUpdateTime time.Time
	UpdateCount    int
	Source         string // "api" или "blockchain"
}

// PoolCache предоставляет потокобезопасное временное хранилище для пулов
type PoolCache struct {
	entries map[string]CacheEntry
	mu      sync.RWMutex
	logger  *zap.Logger
	ttl     time.Duration

	// Метрики кэша
	stats CacheStats

	// Канал для остановки очистки
	stopCleanup chan struct{}
}

// CacheStats содержит метрики использования кэша
type CacheStats struct {
	Hits        int64
	Misses      int64
	Expirations int64
	Updates     int64
	mu          sync.Mutex
}

// NewPoolCache создает новый экземпляр кэша пулов
func NewPoolCache(logger *zap.Logger) *PoolCache {
	pc := &PoolCache{
		entries:     make(map[string]CacheEntry),
		logger:      logger.Named("pool-cache"),
		ttl:         defaultCacheTTL,
		stopCleanup: make(chan struct{}),
	}

	// Запускаем горутину для периодической очистки
	go pc.startCleanupRoutine()

	return pc
}

// AddPool добавляет или обновляет пул в кэше
func (pc *PoolCache) AddPool(pool *Pool) error {
	if err := pc.validatePoolForCache(pool); err != nil {
		return fmt.Errorf("pool validation failed: %w", err)
	}

	pc.mu.Lock()
	defer pc.mu.Unlock()

	now := time.Now()
	entry := CacheEntry{
		Pool:           pool.Clone(), // Создаем копию пула для безопасности
		ExpireAt:       now.Add(pc.ttl),
		LastUpdateTime: now,
		UpdateCount:    1,
		Source:         "api",
	}

	// Создаем ключи для обоих направлений
	keys := pc.generatePoolKeys(pool)

	// Проверяем существующие записи
	for _, key := range keys {
		if existing, exists := pc.entries[key]; exists {
			entry.UpdateCount = existing.UpdateCount + 1
			pc.logPoolUpdate(pool, existing)
		}
		pc.entries[key] = entry
	}

	pc.updateStats(1, 0, 0, 0)

	pc.logger.Debug("pool added to cache",
		zap.String("pool_id", pool.ID.String()),
		zap.String("market_id", pool.MarketID.String()),
		zap.String("token_symbol", pool.TokenSymbol),
		zap.Time("expire_at", entry.ExpireAt))

	return nil
}

// GetPool получает пул из кэша
func (pc *PoolCache) GetPool(baseMint, quoteMint solana.PublicKey) *Pool {
	pc.mu.RLock()
	defer pc.mu.RUnlock()

	now := time.Now()
	keys := []string{
		fmt.Sprintf("%s-%s", baseMint.String(), quoteMint.String()),
		fmt.Sprintf("%s-%s", quoteMint.String(), baseMint.String()),
	}

	for _, key := range keys {
		if entry, exists := pc.entries[key]; exists {
			if now.Before(entry.ExpireAt) {
				pc.updateStats(0, 1, 0, 0)
				return entry.Pool.Clone() // Возвращаем копию
			}
			// Помечаем как устаревшую
			pc.updateStats(0, 0, 1, 0)
		}
	}

	pc.updateStats(0, 0, 0, 1)
	return nil
}

// validatePoolForCache проводит комплексную валидацию пула для кэширования
func (pc *PoolCache) validatePoolForCache(pool *Pool) error {
	if pool == nil {
		return fmt.Errorf("pool cannot be nil")
	}

	// Проверка обязательных полей
	requiredChecks := []struct {
		condition bool
		message   string
	}{
		{pool.ID.IsZero(), "zero pool ID"},
		{pool.MarketID.IsZero(), "zero market ID"},
		{pool.LPMint.IsZero(), "zero LP mint"},
		{pool.TokenSymbol == "", "empty token symbol"},
		{pool.TokenName == "", "empty token name"},
		{!pool.Version.IsValid(), "invalid pool version"},
	}

	for _, check := range requiredChecks {
		if check.condition {
			return fmt.Errorf(check.message)
		}
	}

	// Проверка состояния пула
	if pool.State.Status == PoolStatusUninitialized {
		return fmt.Errorf("pool is uninitialized")
	}

	return nil
}

// Clear очищает все записи из кэша
func (pc *PoolCache) Clear() {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	pc.entries = make(map[string]CacheEntry)
	pc.logger.Info("cache cleared")
}

// SetTTL устанавливает время жизни для записей кэша
func (pc *PoolCache) SetTTL(ttl time.Duration) error {
	if ttl < minCacheTTL || ttl > maxCacheTTL {
		return fmt.Errorf("TTL must be between %v and %v", minCacheTTL, maxCacheTTL)
	}

	pc.mu.Lock()
	defer pc.mu.Unlock()
	pc.ttl = ttl
	return nil
}

// Close останавливает фоновые процессы
func (pc *PoolCache) Close() {
	close(pc.stopCleanup)
}

// Вспомогательные методы

func (pc *PoolCache) startCleanupRoutine() {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			pc.removeExpired()
		case <-pc.stopCleanup:
			return
		}
	}
}

func (pc *PoolCache) removeExpired() {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	now := time.Now()
	for key, entry := range pc.entries {
		if now.After(entry.ExpireAt) {
			delete(pc.entries, key)
			pc.logger.Debug("removed expired pool from cache",
				zap.String("pool_id", entry.Pool.ID.String()),
				zap.String("key", key))
		}
	}
}

func (pc *PoolCache) generatePoolKeys(pool *Pool) []string {
	// Генерируем ключи для различных комбинаций токенов
	keys := make([]string, 0, 2)

	// Base-Quote направление
	if !pool.BaseMint.IsZero() && !pool.QuoteMint.IsZero() {
		keys = append(keys, fmt.Sprintf("%s-%s",
			pool.BaseMint.String(),
			pool.QuoteMint.String()))
	}

	// Quote-Base направление
	if !pool.QuoteMint.IsZero() && !pool.BaseMint.IsZero() {
		keys = append(keys, fmt.Sprintf("%s-%s",
			pool.QuoteMint.String(),
			pool.BaseMint.String()))
	}

	return keys
}

func (pc *PoolCache) logPoolUpdate(newPool *Pool, existing CacheEntry) {
	pc.logger.Debug("updating existing pool",
		zap.String("pool_id", newPool.ID.String()),
		zap.Int("update_count", existing.UpdateCount+1),
		zap.Time("last_update", existing.LastUpdateTime),
		zap.String("token_symbol", newPool.TokenSymbol))
}

func (pc *PoolCache) updateStats(updates, hits, expirations, misses int64) {
	pc.stats.mu.Lock()
	defer pc.stats.mu.Unlock()

	pc.stats.Updates += updates
	pc.stats.Hits += hits
	pc.stats.Expirations += expirations
	pc.stats.Misses += misses
}

// GetStats возвращает текущую статистику кэша
func (pc *PoolCache) GetStats() map[string]interface{} {
	pc.mu.RLock()
	defer pc.mu.RUnlock()

	pc.stats.mu.Lock()
	defer pc.stats.mu.Unlock()

	now := time.Now()
	var expired, valid int

	for _, entry := range pc.entries {
		if now.After(entry.ExpireAt) {
			expired++
		} else {
			valid++
		}
	}

	return map[string]interface{}{
		"total_entries":   len(pc.entries),
		"valid_entries":   valid,
		"expired_entries": expired,
		"ttl_seconds":     pc.ttl.Seconds(),
		"hits":            pc.stats.Hits,
		"misses":          pc.stats.Misses,
		"updates":         pc.stats.Updates,
		"expirations":     pc.stats.Expirations,
	}
}
