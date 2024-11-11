// internal/dex/raydium/pool_cache.go
package raydium

import (
	"fmt"
	"sync"
	"time"

	"github.com/gagliardetto/solana-go"
	"go.uber.org/zap"
)

// CacheEntry представляет запись в кэше с временем жизни
type CacheEntry struct {
	Pool     *Pool
	ExpireAt time.Time
}

// PoolCache предоставляет временное хранилище для пулов
type PoolCache struct {
	entries map[string]CacheEntry
	mu      sync.RWMutex
	logger  *zap.Logger
	ttl     time.Duration
}

// NewPoolCache создает новый экземпляр кэша пулов
func NewPoolCache(logger *zap.Logger) *PoolCache {
	return &PoolCache{
		entries: make(map[string]CacheEntry),
		logger:  logger.Named("pool-cache"),
		ttl:     15 * time.Minute, // Дефолтное время жизни кэша
	}
}

// SetTTL устанавливает время жизни для записей кэша
func (pc *PoolCache) SetTTL(ttl time.Duration) {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	pc.ttl = ttl
}

// AddPool добавляет пул в кэш
func (pc *PoolCache) AddPool(pool *Pool) error {
	if pool == nil {
		return fmt.Errorf("cannot add nil pool")
	}
	if err := pc.validatePool(pool); err != nil {
		return fmt.Errorf("invalid pool data: %w", err)
	}

	pc.mu.Lock()
	defer pc.mu.Unlock()

	// Создаем ключи для обоих направлений (base->quote и quote->base)
	keys := []string{
		fmt.Sprintf("%s-%s", pool.BaseMint.String(), pool.QuoteMint.String()),
		fmt.Sprintf("%s-%s", pool.QuoteMint.String(), pool.BaseMint.String()),
	}

	entry := CacheEntry{
		Pool:     pool,
		ExpireAt: time.Now().Add(pc.ttl),
	}

	// Сохраняем пул под обоими ключами
	for _, key := range keys {
		pc.entries[key] = entry
	}

	pc.logger.Debug("pool added to cache",
		zap.String("pool_id", pool.ID.String()),
		zap.String("base_mint", pool.BaseMint.String()),
		zap.String("quote_mint", pool.QuoteMint.String()),
		zap.Time("expire_at", entry.ExpireAt))

	return nil
}

// GetPool получает пул из кэша
func (pc *PoolCache) GetPool(baseMint, quoteMint solana.PublicKey) *Pool {
	pc.mu.RLock()
	defer pc.mu.RUnlock()

	now := time.Now()

	// Пробуем оба варианта ключей
	keys := []string{
		fmt.Sprintf("%s-%s", baseMint.String(), quoteMint.String()),
		fmt.Sprintf("%s-%s", quoteMint.String(), baseMint.String()),
	}

	for _, key := range keys {
		if entry, exists := pc.entries[key]; exists {
			// Проверяем не истекло ли время жизни записи
			if now.Before(entry.ExpireAt) {
				return entry.Pool
			}
			// Удаляем устаревшую запись
			delete(pc.entries, key)
		}
	}

	return nil
}

// Clear очищает все записи из кэша
func (pc *PoolCache) Clear() {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	pc.entries = make(map[string]CacheEntry)
	pc.logger.Debug("cache cleared")
}

// RemoveExpired удаляет все истекшие записи из кэша
func (pc *PoolCache) RemoveExpired() {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	now := time.Now()
	for key, entry := range pc.entries {
		if now.After(entry.ExpireAt) {
			delete(pc.entries, key)
			pc.logger.Debug("removed expired pool from cache",
				zap.String("pool_id", entry.Pool.ID.String()))
		}
	}
}

// GetStats возвращает статистику использования кэша
func (pc *PoolCache) GetStats() map[string]interface{} {
	pc.mu.RLock()
	defer pc.mu.RUnlock()

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
	}
}

// validatePool проверяет валидность пула
func (pc *PoolCache) validatePool(pool *Pool) error {
	if pool.ID.IsZero() {
		return fmt.Errorf("zero pool ID")
	}
	if pool.Authority.IsZero() {
		return fmt.Errorf("zero authority")
	}
	if pool.BaseMint.IsZero() {
		return fmt.Errorf("zero base mint")
	}
	if pool.QuoteMint.IsZero() {
		return fmt.Errorf("zero quote mint")
	}
	if pool.BaseVault.IsZero() {
		return fmt.Errorf("zero base vault")
	}
	if pool.QuoteVault.IsZero() {
		return fmt.Errorf("zero quote vault")
	}
	if !pool.Version.IsValid() {
		return fmt.Errorf("invalid pool version")
	}
	return nil
}
