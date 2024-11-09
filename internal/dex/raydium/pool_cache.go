// internal/dex/raydium/pool_cache.go
package raydium

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sync"

	"github.com/gagliardetto/solana-go"
	"go.uber.org/zap"
)

type PoolCache struct {
	pools     map[string]*Pool
	jsonPools *PoolList
	mu        sync.RWMutex
	logger    *zap.Logger
}

func NewPoolCache(logger *zap.Logger) *PoolCache {
	return &PoolCache{
		pools:  make(map[string]*Pool),
		logger: logger,
	}
}

func (pc *PoolCache) LoadPoolsFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read pools file: %w", err)
	}

	var poolList PoolList
	if err := json.Unmarshal(data, &poolList); err != nil {
		return fmt.Errorf("failed to unmarshal pools: %w", err)
	}

	pc.mu.Lock()
	pc.jsonPools = &poolList
	pc.mu.Unlock()

	return nil
}

// GetPoolsCount возвращает количество пулов в кэше
func (pc *PoolCache) GetPoolsCount() int {
	pc.mu.RLock()
	defer pc.mu.RUnlock()

	if pc.jsonPools == nil {
		return len(pc.pools)
	}
	return len(pc.jsonPools.Official) + len(pc.jsonPools.Unofficial)
}

// IsLoaded проверяет загружены ли пулы из файла
func (pc *PoolCache) IsLoaded() bool {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	return pc.jsonPools != nil
}

// AddPool добавляет пул в кэш
func (pc *PoolCache) AddPool(pool *Pool) error {
	if pool == nil {
		return fmt.Errorf("cannot add nil pool")
	}
	if !pc.isValidPool(pool) {
		return fmt.Errorf("invalid pool data")
	}

	cacheKey := fmt.Sprintf("%s-%s", pool.BaseMint.String(), pool.QuoteMint.String())
	pc.mu.Lock()
	pc.pools[cacheKey] = pool
	pc.mu.Unlock()
	return nil
}

// GetPool получает пул из кэша
func (pc *PoolCache) GetPool(baseMint, quoteMint solana.PublicKey) *Pool {
	cacheKey := fmt.Sprintf("%s-%s", baseMint.String(), quoteMint.String())
	pc.mu.RLock()
	pool := pc.pools[cacheKey]
	pc.mu.RUnlock()
	return pool
}

// findPoolInJson ищет пул в загруженном JSON файле
func (pc *PoolCache) findPoolInJSON(baseMint, quoteMint solana.PublicKey) *Pool {
	pc.mu.RLock()
	defer pc.mu.RUnlock()

	if pc.jsonPools == nil {
		return nil
	}

	// Функция для поиска в списке пулов
	findInPoolList := func(pools []PoolJSONInfo) *Pool {
		for _, info := range pools {
			if (info.BaseMint == baseMint.String() && info.QuoteMint == quoteMint.String()) ||
				(info.BaseMint == quoteMint.String() && info.QuoteMint == baseMint.String()) {

				pool := pc.convertJSONToPool(info)
				if pool != nil && pc.isValidPool(pool) {
					pc.logger.Debug("found valid pool in JSON",
						zap.String("pool_id", pool.ID.String()),
						zap.String("base_mint", pool.BaseMint.String()),
						zap.String("quote_mint", pool.QuoteMint.String()))
					return pool
				}
			}
		}
		return nil
	}

	// Сначала ищем в официальных пулах
	if pool := findInPoolList(pc.jsonPools.Official); pool != nil {
		return pool
	}

	// Затем в неофициальных
	return findInPoolList(pc.jsonPools.Unofficial)
}

func (pc *PoolCache) convertJSONToPool(info PoolJSONInfo) *Pool {
	defer func() {
		if r := recover(); r != nil {
			pc.logger.Error("failed to convert pool info",
				zap.String("id", info.ID),
				zap.Any("panic", r))
		}
	}()

	// Создаем безопасные конвертеры для uint8
	safeUint8 := func(n int) (uint8, error) {
		if n < 0 || n > math.MaxUint8 {
			return 0, fmt.Errorf("value %d outside uint8 range", n)
		}
		return uint8(n), nil
	}

	// Безопасно конвертируем decimals
	baseDecimals, err := safeUint8(info.BaseDecimals)
	if err != nil {
		pc.logger.Error("invalid base decimals",
			zap.String("pool_id", info.ID),
			zap.Int("decimals", info.BaseDecimals),
			zap.Error(err))
		return nil
	}

	quoteDecimals, err := safeUint8(info.QuoteDecimals)
	if err != nil {
		pc.logger.Error("invalid quote decimals",
			zap.String("pool_id", info.ID),
			zap.Int("quote_decimals", info.QuoteDecimals),
			zap.Error(err))
		return nil
	}

	// Проверяем версию пула
	var version PoolVersion
	switch info.Version {
	case 3:
		version = PoolVersionV3
	case 4:
		version = PoolVersionV4
	default:
		pc.logger.Error("unsupported pool version",
			zap.String("pool_id", info.ID),
			zap.Int("version", info.Version))
		return nil
	}

	try := func() *Pool {
		return &Pool{
			ID:            solana.MustPublicKeyFromBase58(info.ID),
			Authority:     solana.MustPublicKeyFromBase58(info.Authority),
			BaseMint:      solana.MustPublicKeyFromBase58(info.BaseMint),
			QuoteMint:     solana.MustPublicKeyFromBase58(info.QuoteMint),
			BaseVault:     solana.MustPublicKeyFromBase58(info.BaseVault),
			QuoteVault:    solana.MustPublicKeyFromBase58(info.QuoteVault),
			BaseDecimals:  baseDecimals,  // Используем безопасно сконвертированное значение
			QuoteDecimals: quoteDecimals, // Используем безопасно сконвертированное значение
			Version:       version,       // Используем проверенную версию
		}
	}

	pool := try()
	if pool == nil {
		return nil
	}

	// Дополнительная проверка созданного пула
	if !pc.isValidPool(pool) {
		pc.logger.Error("created invalid pool",
			zap.String("pool_id", info.ID))
		return nil
	}

	return pool
}

// Добавим метод для валидации версии
func (v PoolVersion) IsValid() bool {
	return v == PoolVersionV3 || v == PoolVersionV4
}

func (pc *PoolCache) isValidPool(pool *Pool) bool {
	return !pool.ID.IsZero() &&
		!pool.Authority.IsZero() &&
		!pool.BaseMint.IsZero() &&
		!pool.QuoteMint.IsZero() &&
		!pool.BaseVault.IsZero() &&
		!pool.QuoteVault.IsZero()
}
