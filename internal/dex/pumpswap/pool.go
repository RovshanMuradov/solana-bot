// =============================
// File: internal/dex/pumpswap/pool.go
// =============================

package pumpswap

import (
	"context"
	"encoding/binary"
	"fmt"
	"github.com/gagliardetto/solana-go/rpc"
	"math"
	"math/big"
	"sync"
	"time"

	"github.com/gagliardetto/solana-go"
	"go.uber.org/zap"

	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc"
)

// Minimum LP supply constant
const MinimumLiquidity uint64 = 1000

// PoolCache хранит найденные пулы для быстрого доступа
type PoolCache struct {
	mutex      sync.RWMutex
	pools      map[string]*PoolInfo // ключ: baseMint:quoteMint
	expiration map[string]time.Time // хранит время истечения срока действия кэша
	ttl        time.Duration        // время жизни записи кэша
}

// NewPoolCache создает новый кэш пулов с указанием TTL
func NewPoolCache(ttl time.Duration) *PoolCache {
	if ttl == 0 {
		// Дефолтное значение - 5 минут
		ttl = 5 * time.Minute
	}

	return &PoolCache{
		pools:      make(map[string]*PoolInfo),
		expiration: make(map[string]time.Time),
		ttl:        ttl,
	}
}

// Get получает пул из кэша, проверяя срок действия
func (pc *PoolCache) Get(baseMint, quoteMint solana.PublicKey) (*PoolInfo, bool) {
	pc.mutex.RLock()
	defer pc.mutex.RUnlock()

	key := makePoolCacheKey(baseMint, quoteMint)
	pool, exists := pc.pools[key]

	// Проверяем, существует ли запись и не истек ли срок ее действия
	if !exists {
		return nil, false
	}

	expiry, hasExpiry := pc.expiration[key]
	if hasExpiry && time.Now().After(expiry) {
		// Срок действия истек, но удалим его позже (при записи)
		return nil, false
	}

	return pool, true
}

// Set добавляет пул в кэш с указанием времени истечения срока действия
func (pc *PoolCache) Set(baseMint, quoteMint solana.PublicKey, pool *PoolInfo) {
	pc.mutex.Lock()
	defer pc.mutex.Unlock()

	// Очищаем истекшие записи при добавлении новых
	pc.cleanExpired()

	key := makePoolCacheKey(baseMint, quoteMint)
	pc.pools[key] = pool
	pc.expiration[key] = time.Now().Add(pc.ttl)
}

// cleanExpired удаляет истекшие записи из кэша
func (pc *PoolCache) cleanExpired() {
	now := time.Now()
	for key, expiry := range pc.expiration {
		if now.After(expiry) {
			delete(pc.pools, key)
			delete(pc.expiration, key)
		}
	}
}

// makePoolCacheKey создает ключ для кэша пулов
func makePoolCacheKey(baseMint, quoteMint solana.PublicKey) string {
	// Всегда сортируем менты для консистентного ключа, независимо от порядка
	if baseMint.String() < quoteMint.String() {
		return fmt.Sprintf("%s:%s", baseMint.String(), quoteMint.String())
	}
	return fmt.Sprintf("%s:%s", quoteMint.String(), baseMint.String())
}

// PoolManager handles operations with PumpSwap pools
type PoolManager struct {
	client     *solbc.Client
	logger     *zap.Logger
	cache      *PoolCache
	programID  solana.PublicKey
	maxRetries int
	retryDelay time.Duration
}

// PoolManagerOptions contains options for creating a PoolManager
type PoolManagerOptions struct {
	CacheTTL   time.Duration
	MaxRetries int
	RetryDelay time.Duration
	ProgramID  solana.PublicKey
}

// DefaultPoolManagerOptions returns default options for PoolManager
func DefaultPoolManagerOptions() PoolManagerOptions {
	return PoolManagerOptions{
		CacheTTL:   5 * time.Minute,
		MaxRetries: 3,
		RetryDelay: time.Second,
		ProgramID:  PumpSwapProgramID,
	}
}

// NewPoolManager creates a new pool manager with options
func NewPoolManager(client *solbc.Client, logger *zap.Logger, opts ...PoolManagerOptions) *PoolManager {
	// Используем дефолтные опции
	defaultOpts := DefaultPoolManagerOptions()

	// Если переданы пользовательские опции, используем их
	var options PoolManagerOptions
	if len(opts) > 0 {
		options = opts[0]
	} else {
		options = defaultOpts
	}

	return &PoolManager{
		client:     client,
		logger:     logger.Named("pool_manager"),
		cache:      NewPoolCache(options.CacheTTL),
		programID:  options.ProgramID,
		maxRetries: options.MaxRetries,
		retryDelay: options.RetryDelay,
	}
}

// GetPoolByProgramAccounts более эффективный метод поиска пула через программные аккаунты
func (pm *PoolManager) GetPoolByProgramAccounts(
	ctx context.Context,
	baseMint, quoteMint solana.PublicKey,
) (*PoolInfo, error) {
	// Используем определение возможных PDA и GetMultipleAccounts
	// для поиска пула вместо прямого вызова GetProgramAccounts

	// Формируем список возможных адресов пулов
	possiblePoolAddresses := []solana.PublicKey{}

	// Создаем временную конфигурацию для получения адреса пула
	cfg := &Config{
		ProgramID: PumpSwapProgramID,
		BaseMint:  baseMint,
		QuoteMint: quoteMint,
	}

	// Перебираем возможные индексы и создателей пулов
	possibleCreators := []solana.PublicKey{
		PumpSwapProgramID, // Самый вероятный создатель - сама программа
		solana.MustPublicKeyFromBase58("8LWu7QM2dGR1G8nKDHthckea57bkCzXyBTAKPJUBDHo8"), // Admin из IDL
	}

	// Проверяем сначала наиболее вероятные индексы (0-9)
	for index := uint16(0); index < 10; index++ {
		for _, creator := range possibleCreators {
			// Получаем адрес пула для этой комбинации
			poolAddr, _, err := cfg.DerivePoolAddress(index, creator)
			if err != nil {
				continue
			}
			possiblePoolAddresses = append(possiblePoolAddresses, poolAddr)
		}
	}

	// Получаем информацию о нескольких аккаунтах за один запрос
	accountsResult, err := pm.client.GetMultipleAccounts(ctx, possiblePoolAddresses)
	if err != nil {
		return nil, fmt.Errorf("failed to get multiple accounts: %w", err)
	}

	// Проверяем, есть ли результаты
	if accountsResult == nil || accountsResult.Value == nil || len(accountsResult.Value) == 0 {
		return nil, fmt.Errorf("no pool accounts found for the specified mints")
	}

	// Обрабатываем полученные аккаунты
	for i, account := range accountsResult.Value {
		if account == nil {
			continue // Аккаунт не существует
		}

		// Проверяем, что аккаунт принадлежит PumpSwap программе
		if !account.Owner.Equals(PumpSwapProgramID) {
			continue
		}

		// Проверяем дискриминатор (первые 8 байт)
		accountData := account.Data.GetBinary()
		if len(accountData) < 8 {
			continue
		}

		// Сравниваем дискриминаторы
		discriminator := accountData[:8]
		match := true
		for j := 0; j < 8 && j < len(PoolDiscriminator); j++ {
			if discriminator[j] != PoolDiscriminator[j] {
				match = false
				break
			}
		}

		if !match {
			continue
		}

		// Пытаемся распарсить данные пула
		pool, err := ParsePool(accountData)
		if err != nil {
			pm.logger.Debug("Failed to parse pool data",
				zap.String("pool_address", possiblePoolAddresses[i].String()),
				zap.Error(err))
			continue
		}

		// Проверяем, соответствует ли пул искомой паре токенов
		// Учитываем возможность обратного порядка токенов
		if (pool.BaseMint.Equals(baseMint) && pool.QuoteMint.Equals(quoteMint)) ||
			(pool.BaseMint.Equals(quoteMint) && pool.QuoteMint.Equals(baseMint)) {
			// Нашли подходящий пул
			poolInfo, err := pm.FetchPoolInfo(ctx, possiblePoolAddresses[i])
			if err != nil {
				pm.logger.Error("Failed to fetch pool info",
					zap.String("pool_address", possiblePoolAddresses[i].String()),
					zap.Error(err))
				continue
			}

			// Сохраняем в кэш
			if pm.cache != nil {
				pm.cache.Set(baseMint, quoteMint, poolInfo)
			}

			pm.logger.Info("Found PumpSwap pool via program accounts",
				zap.String("pool_address", possiblePoolAddresses[i].String()),
				zap.String("base_mint", pool.BaseMint.String()),
				zap.String("quote_mint", pool.QuoteMint.String()))

			return poolInfo, nil
		}
	}

	// Если не нашли в первых 10 индексах, проверим следующие (10-99)
	// Только если контекст ещё не истёк
	if ctx.Err() == nil {
		// Формируем список дополнительных возможных адресов
		additionalAddresses := []solana.PublicKey{}

		for index := uint16(10); index < 100; index++ {
			for _, creator := range possibleCreators {
				poolAddr, _, err := cfg.DerivePoolAddress(index, creator)
				if err != nil {
					continue
				}
				additionalAddresses = append(additionalAddresses, poolAddr)
			}
		}

		// Получаем информацию о дополнительных аккаунтах
		if len(additionalAddresses) > 0 {
			additionalResult, err := pm.client.GetMultipleAccounts(ctx, additionalAddresses)
			if err == nil && additionalResult != nil && additionalResult.Value != nil {
				for i, account := range additionalResult.Value {
					if account == nil {
						continue
					}

					if !account.Owner.Equals(PumpSwapProgramID) {
						continue
					}

					accountData := account.Data.GetBinary()
					if len(accountData) < 8 {
						continue
					}

					// Сравниваем дискриминаторы
					discriminator := accountData[:8]
					match := true
					for j := 0; j < 8 && j < len(PoolDiscriminator); j++ {
						if discriminator[j] != PoolDiscriminator[j] {
							match = false
							break
						}
					}

					if !match {
						continue
					}

					pool, err := ParsePool(accountData)
					if err != nil {
						continue
					}

					if (pool.BaseMint.Equals(baseMint) && pool.QuoteMint.Equals(quoteMint)) ||
						(pool.BaseMint.Equals(quoteMint) && pool.QuoteMint.Equals(baseMint)) {

						poolInfo, err := pm.FetchPoolInfo(ctx, additionalAddresses[i])
						if err != nil {
							continue
						}

						// Сохраняем в кэш
						if pm.cache != nil {
							pm.cache.Set(baseMint, quoteMint, poolInfo)
						}

						pm.logger.Info("Found PumpSwap pool in extended search",
							zap.String("pool_address", additionalAddresses[i].String()),
							zap.String("base_mint", pool.BaseMint.String()),
							zap.String("quote_mint", pool.QuoteMint.String()))

						return poolInfo, nil
					}
				}
			}
		}
	}

	return nil, fmt.Errorf("no matching pool found for base mint %s and quote mint %s",
		baseMint.String(), quoteMint.String())
}

// FindPool finds a pool for the given token pair using multiple strategies
func (pm *PoolManager) FindPool(
	ctx context.Context,
	baseMint, quoteMint solana.PublicKey,
) (*PoolInfo, error) {
	// 1. Сначала проверяем кэш (для обоих направлений пары)
	if pool, found := pm.cache.Get(baseMint, quoteMint); found {
		pm.logger.Debug("Found pool in cache",
			zap.String("base_mint", baseMint.String()),
			zap.String("quote_mint", quoteMint.String()))
		return pool, nil
	}

	// Для проверки в обратном порядке
	if pool, found := pm.cache.Get(quoteMint, baseMint); found {
		pm.logger.Debug("Found pool in cache (reversed order)",
			zap.String("base_mint", quoteMint.String()),
			zap.String("quote_mint", baseMint.String()))
		// Меняем местами токены в результате для консистентности с запросом
		pool.BaseMint, pool.QuoteMint = pool.QuoteMint, pool.BaseMint
		pool.BaseReserves, pool.QuoteReserves = pool.QuoteReserves, pool.BaseReserves
		pool.PoolBaseTokenAccount, pool.PoolQuoteTokenAccount = pool.PoolQuoteTokenAccount, pool.PoolBaseTokenAccount
		return pool, nil
	}

	// 2. Попробуем найти с помощью эффективного поиска
	pool, err := pm.findPoolEfficiently(ctx, baseMint, quoteMint)
	if err == nil && pool != nil {
		// Сохраняем в кэш
		pm.cache.Set(baseMint, quoteMint, pool)
		return pool, nil
	}

	// 3. Пробуем найти в обратном порядке токенов
	pool, err = pm.findPoolEfficiently(ctx, quoteMint, baseMint)
	if err == nil && pool != nil {
		// Меняем местами токены в результате для консистентности с запросом
		pool.BaseMint, pool.QuoteMint = pool.QuoteMint, pool.BaseMint
		pool.BaseReserves, pool.QuoteReserves = pool.QuoteReserves, pool.BaseReserves
		pool.PoolBaseTokenAccount, pool.PoolQuoteTokenAccount = pool.PoolQuoteTokenAccount, pool.PoolBaseTokenAccount

		// Сохраняем в кэш в обоих направлениях
		pm.cache.Set(baseMint, quoteMint, pool)
		return pool, nil
	}

	// 4. Резервный метод: прямой поиск через GetProgramAccounts
	pool, err = pm.findPoolByProgramAccounts(ctx, baseMint, quoteMint)
	if err == nil && pool != nil {
		pm.cache.Set(baseMint, quoteMint, pool)
		return pool, nil
	}

	// Если все методы не нашли пул
	return nil, fmt.Errorf("no pool found for base mint %s and quote mint %s",
		baseMint.String(), quoteMint.String())
}

// findPoolEfficiently пытается найти пул используя эффективные стратегии
func (pm *PoolManager) findPoolEfficiently(
	ctx context.Context,
	baseMint, quoteMint solana.PublicKey,
) (*PoolInfo, error) {
	// Метод 1: Проверяем наиболее вероятные пулы по индексам и создателям
	possiblePoolAddresses := pm.generateLikelyPoolAddresses(baseMint, quoteMint)

	// Разбиваем на более мелкие батчи для более эффективного выполнения запросов
	// и избежания превышения лимитов RPC
	const batchSize = 25
	for i := 0; i < len(possiblePoolAddresses); i += batchSize {
		end := i + batchSize
		if end > len(possiblePoolAddresses) {
			end = len(possiblePoolAddresses)
		}

		batch := possiblePoolAddresses[i:end]

		// Получаем множество аккаунтов за один запрос
		accountsResult, err := pm.client.GetMultipleAccounts(ctx, batch)
		if err != nil {
			pm.logger.Debug("Failed to get multiple accounts",
				zap.Error(err),
				zap.Int("batch_start", i),
				zap.Int("batch_end", end))
			continue
		}

		// Проверяем полученные результаты
		pool := pm.processAccountBatch(ctx, accountsResult, batch, baseMint, quoteMint)
		if pool != nil {
			return pool, nil
		}
	}

	return nil, fmt.Errorf("no pool found with efficient search")
}

// generateLikelyPoolAddresses генерирует список наиболее вероятных адресов пулов
func (pm *PoolManager) generateLikelyPoolAddresses(
	baseMint, quoteMint solana.PublicKey,
) []solana.PublicKey {
	var possiblePoolAddresses []solana.PublicKey

	// Список потенциальных создателей пулов, начиная с наиболее вероятных
	possibleCreators := []solana.PublicKey{
		PumpSwapProgramID, // Сама программа как создатель (наиболее вероятный)
		solana.MustPublicKeyFromBase58("8LWu7QM2dGR1G8nKDHthckea57bkCzXyBTAKPJUBDHo8"), // Admin из IDL
	}

	// Временная конфигурация для деривации адреса пула
	cfg := &Config{
		ProgramID: pm.programID,
		BaseMint:  baseMint,
		QuoteMint: quoteMint,
	}

	// Сначала проверяем наиболее вероятные индексы (0-10)
	for index := uint16(0); index < 10; index++ {
		for _, creator := range possibleCreators {
			poolAddr, _, err := cfg.DerivePoolAddress(index, creator)
			if err != nil {
				continue
			}
			possiblePoolAddresses = append(possiblePoolAddresses, poolAddr)
		}
	}

	// Затем проверяем последующие индексы (10-100) с меньшим приоритетом
	for index := uint16(10); index < 100; index += 5 { // Проверяем каждый 5-й индекс для экономии RPC вызовов
		for _, creator := range possibleCreators[:1] { // Используем только первого создателя для экономии
			poolAddr, _, err := cfg.DerivePoolAddress(index, creator)
			if err != nil {
				continue
			}
			possiblePoolAddresses = append(possiblePoolAddresses, poolAddr)
		}
	}

	return possiblePoolAddresses
}

// processAccountBatch обрабатывает пакет аккаунтов в поисках действительного пула
func (pm *PoolManager) processAccountBatch(
	ctx context.Context,
	accountsResult *rpc.GetMultipleAccountsResult,
	addresses []solana.PublicKey,
	baseMint, quoteMint solana.PublicKey,
) *PoolInfo {
	if accountsResult == nil || accountsResult.Value == nil {
		return nil
	}

	for i, account := range accountsResult.Value {
		if account == nil {
			continue // Пропускаем несуществующие аккаунты
		}

		// Проверяем, что владелец - программа PumpSwap
		if !account.Owner.Equals(pm.programID) {
			continue
		}

		// Проверяем данные аккаунта
		accountData := account.Data.GetBinary()
		if len(accountData) < 8 {
			continue
		}

		// Проверяем дискриминатор
		if !isPoolDiscriminator(accountData[:8]) {
			continue
		}

		// Пытаемся распарсить пул
		pool, err := ParsePool(accountData)
		if err != nil {
			pm.logger.Debug("Failed to parse pool data",
				zap.String("pool_address", addresses[i].String()),
				zap.Error(err))
			continue
		}

		// Проверяем совпадение минтов
		if (pool.BaseMint.Equals(baseMint) && pool.QuoteMint.Equals(quoteMint)) ||
			(pool.BaseMint.Equals(quoteMint) && pool.QuoteMint.Equals(baseMint)) {

			// Нашли пул, получаем полную информацию
			poolInfo, err := pm.FetchPoolInfo(ctx, addresses[i])
			if err != nil {
				pm.logger.Debug("Failed to fetch pool info",
					zap.String("pool_address", addresses[i].String()),
					zap.Error(err))
				continue
			}

			// Проверяем, что пул активен и имеет ликвидность
			if poolInfo.BaseReserves == 0 || poolInfo.QuoteReserves == 0 {
				pm.logger.Debug("Pool has no liquidity",
					zap.String("pool_address", addresses[i].String()),
					zap.Uint64("base_reserves", poolInfo.BaseReserves),
					zap.Uint64("quote_reserves", poolInfo.QuoteReserves))
				continue
			}

			pm.logger.Info("Found active PumpSwap pool",
				zap.String("pool_address", addresses[i].String()),
				zap.String("base_mint", pool.BaseMint.String()),
				zap.String("quote_mint", pool.QuoteMint.String()),
				zap.Uint64("base_reserves", poolInfo.BaseReserves),
				zap.Uint64("quote_reserves", poolInfo.QuoteReserves))

			return poolInfo
		}
	}

	return nil
}

// isPoolDiscriminator проверяет соответствие дискриминатора пула
func isPoolDiscriminator(discriminator []byte) bool {
	if len(discriminator) != 8 || len(PoolDiscriminator) != 8 {
		return false
	}

	for i := 0; i < 8; i++ {
		if discriminator[i] != PoolDiscriminator[i] {
			return false
		}
	}

	return true
}

// findPoolByProgramAccounts ищет пул с использованием GetProgramAccounts с фильтрами
func (pm *PoolManager) findPoolByProgramAccounts(
	ctx context.Context,
	baseMint, quoteMint solana.PublicKey,
) (*PoolInfo, error) {
	// Создаем опции запроса с фильтрами по дискриминатору
	opts := &rpc.GetProgramAccountsOpts{
		Commitment: rpc.CommitmentConfirmed,
		Encoding:   solana.EncodingBase64,
		Filters: []rpc.RPCFilter{
			{
				Memcmp: &rpc.RPCFilterMemcmp{
					Offset: 0,
					Bytes:  PoolDiscriminator,
				},
			},
		},
	}

	// Используем метод из адаптера-клиента
	accounts, err := pm.client.GetProgramAccountsWithOpts(ctx, pm.programID, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to get program accounts: %w", err)
	}

	// Перебираем все пулы и ищем совпадение минтов
	for _, account := range accounts {
		poolData := account.Account.Data.GetBinary()
		pool, err := ParsePool(poolData)
		if err != nil {
			continue
		}

		// Проверяем, соответствует ли пул искомой паре токенов
		if (pool.BaseMint.Equals(baseMint) && pool.QuoteMint.Equals(quoteMint)) ||
			(pool.BaseMint.Equals(quoteMint) && pool.QuoteMint.Equals(baseMint)) {

			// Нашли подходящий пул
			poolInfo, err := pm.FetchPoolInfo(ctx, account.Pubkey)
			if err != nil {
				continue
			}

			// Проверяем наличие ликвидности
			if poolInfo.BaseReserves == 0 || poolInfo.QuoteReserves == 0 {
				continue
			}

			pm.logger.Info("Found PumpSwap pool via GetProgramAccounts",
				zap.String("pool_address", account.Pubkey.String()),
				zap.String("base_mint", pool.BaseMint.String()),
				zap.String("quote_mint", pool.QuoteMint.String()))

			return poolInfo, nil
		}
	}

	return nil, fmt.Errorf("no matching pool found via GetProgramAccounts")
}

// FetchPoolInfo fetches detailed pool information
func (pm *PoolManager) FetchPoolInfo(
	ctx context.Context,
	poolAddress solana.PublicKey,
) (*PoolInfo, error) {
	// Get pool account data
	accountInfo, err := pm.client.GetAccountInfo(ctx, poolAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to get pool account: %w", err)
	}

	if accountInfo == nil || accountInfo.Value == nil {
		return nil, fmt.Errorf("pool account not found: %s", poolAddress.String())
	}

	// Parse pool data
	poolData := accountInfo.Value.Data.GetBinary()
	pool, err := ParsePool(poolData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse pool data: %w", err)
	}

	// Get global config to get the fee information
	globalConfig, _, err := solana.FindProgramAddress(
		[][]byte{[]byte("global_config")},
		pm.programID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to derive global config address: %w", err)
	}

	// Get global config account data
	globalAccountInfo, err := pm.client.GetAccountInfo(ctx, globalConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to get global config account: %w", err)
	}

	if globalAccountInfo == nil || globalAccountInfo.Value == nil {
		return nil, fmt.Errorf("global config account not found: %s", globalConfig.String())
	}

	// Parse global config data
	globalData := globalAccountInfo.Value.Data.GetBinary()
	config, err := ParseGlobalConfig(globalData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse global config data: %w", err)
	}

	// Получаем информацию о токеновых аккаунтах одним запросом
	tokenAccounts := []solana.PublicKey{
		pool.PoolBaseTokenAccount,
		pool.PoolQuoteTokenAccount,
	}

	tokenAccountsInfo, err := pm.client.GetMultipleAccounts(ctx, tokenAccounts)
	if err != nil {
		return nil, fmt.Errorf("failed to get token accounts: %w", err)
	}

	// Parse token accounts to get reserves
	var baseReserves, quoteReserves uint64

	if tokenAccountsInfo != nil && tokenAccountsInfo.Value != nil {
		if len(tokenAccountsInfo.Value) > 0 && tokenAccountsInfo.Value[0] != nil {
			// Extract token balance from the account data
			// SPL Token account data structure:
			// - Mint: 32 bytes (offset 0)
			// - Owner: 32 bytes (offset 32)
			// - Amount: 8 bytes (offset 64)
			baseData := tokenAccountsInfo.Value[0].Data.GetBinary()
			if len(baseData) >= 72 {
				// SPL Token amounts are stored in little-endian format
				baseReserves = binary.LittleEndian.Uint64(baseData[64:72])
			}
		}

		if len(tokenAccountsInfo.Value) > 1 && tokenAccountsInfo.Value[1] != nil {
			quoteData := tokenAccountsInfo.Value[1].Data.GetBinary()
			if len(quoteData) >= 72 {
				// SPL Token amounts are stored in little-endian format
				quoteReserves = binary.LittleEndian.Uint64(quoteData[64:72])
			}
		}
	}

	// Create pool info
	poolInfo := &PoolInfo{
		Address:               poolAddress,
		BaseMint:              pool.BaseMint,
		QuoteMint:             pool.QuoteMint,
		BaseReserves:          baseReserves,
		QuoteReserves:         quoteReserves,
		LPSupply:              pool.LPSupply,
		FeesBasisPoints:       config.LPFeeBasisPoints,
		ProtocolFeeBPS:        config.ProtocolFeeBasisPoints,
		LPMint:                pool.LPMint,
		PoolBaseTokenAccount:  pool.PoolBaseTokenAccount,
		PoolQuoteTokenAccount: pool.PoolQuoteTokenAccount,
	}

	return poolInfo, nil
}

// CalculateSwapQuote calculates the expected output amount for a swap
func (pm *PoolManager) CalculateSwapQuote(
	pool *PoolInfo,
	inputAmount uint64,
	isBaseToQuote bool,
) (uint64, float64) {
	var outputAmount uint64
	var price float64

	// Calculate fees
	feeFactor := 1.0 - (float64(pool.FeesBasisPoints) / 10000.0)

	if isBaseToQuote {
		// Selling base tokens to get quote tokens (e.g., SOL to TOKEN)
		x := new(big.Float).SetUint64(pool.BaseReserves)
		y := new(big.Float).SetUint64(pool.QuoteReserves)
		a := new(big.Float).SetUint64(inputAmount)

		// Apply fee to input amount
		a = new(big.Float).Mul(a, big.NewFloat(feeFactor))

		// Calculate: outputAmount = y * a / (x + a)
		numerator := new(big.Float).Mul(y, a)
		denominator := new(big.Float).Add(x, a)
		result := new(big.Float).Quo(numerator, denominator)

		// Convert result to uint64
		resultUint64, _ := result.Uint64()
		outputAmount = resultUint64

		// Calculate price: outputAmount / inputAmount
		if inputAmount > 0 {
			price = float64(outputAmount) / float64(inputAmount)
		}
	} else {
		// Selling quote tokens to get base tokens (e.g., TOKEN to SOL)
		x := new(big.Float).SetUint64(pool.QuoteReserves)
		y := new(big.Float).SetUint64(pool.BaseReserves)
		a := new(big.Float).SetUint64(inputAmount)

		// Apply fee to input amount
		a = new(big.Float).Mul(a, big.NewFloat(feeFactor))

		// Calculate: outputAmount = y * a / (x + a)
		numerator := new(big.Float).Mul(y, a)
		denominator := new(big.Float).Add(x, a)
		result := new(big.Float).Quo(numerator, denominator)

		// Convert result to uint64
		resultUint64, _ := result.Uint64()
		outputAmount = resultUint64

		// Calculate price: inputAmount / outputAmount
		if outputAmount > 0 {
			price = float64(inputAmount) / float64(outputAmount)
		}
	}

	return outputAmount, price
}

// CalculateSlippage calculates the slippage for a swap
func (pm *PoolManager) CalculateSlippage(
	pool *PoolInfo,
	inputAmount uint64,
	isBaseToQuote bool,
) float64 {
	var initialPrice, finalPrice float64

	if isBaseToQuote {
		// Initial price (base -> quote)
		initialPrice = float64(pool.QuoteReserves) / float64(pool.BaseReserves)

		// Calculate output amount
		outputAmount, _ := pm.CalculateSwapQuote(pool, inputAmount, true)

		// Final price after swap
		finalPrice = float64(pool.QuoteReserves-outputAmount) /
			float64(pool.BaseReserves+inputAmount)
	} else {
		// Initial price (quote -> base)
		initialPrice = float64(pool.BaseReserves) / float64(pool.QuoteReserves)

		// Calculate output amount
		outputAmount, _ := pm.CalculateSwapQuote(pool, inputAmount, false)

		// Final price after swap
		finalPrice = float64(pool.BaseReserves-outputAmount) /
			float64(pool.QuoteReserves+inputAmount)
	}

	// Calculate slippage as percentage
	slippage := math.Abs(finalPrice-initialPrice) / initialPrice * 100

	return slippage
}

// FindPoolWithRetry attempts to find a pool with retries
func (pm *PoolManager) FindPoolWithRetry(
	ctx context.Context,
	baseMint, quoteMint solana.PublicKey,
	maxRetries int,
	retryDelay time.Duration,
) (*PoolInfo, error) {
	if maxRetries <= 0 {
		maxRetries = pm.maxRetries
	}

	if retryDelay <= 0 {
		retryDelay = pm.retryDelay
	}

	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		// Создаем контекст с таймаутом для каждой попытки
		searchCtx, cancel := context.WithTimeout(ctx, retryDelay*2)

		// Try to find the pool
		poolInfo, err := pm.FindPool(searchCtx, baseMint, quoteMint)
		cancel() // Отменяем поисковый контекст

		if err == nil {
			return poolInfo, nil
		}

		lastErr = err

		pm.logger.Debug("Failed to find pool, retrying",
			zap.Int("attempt", attempt+1),
			zap.Int("max_retries", maxRetries),
			zap.Duration("retry_delay", retryDelay),
			zap.Error(err))

		// Wait before retry
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(retryDelay):
			// Continue with next attempt
		}
	}

	return nil, fmt.Errorf("failed to find pool after %d attempts: %w", maxRetries, lastErr)
}

// DerivePoolAddress вычисляет PDA для пула с заданными параметрами.
func (cfg *Config) DerivePoolAddress(index uint16, creator solana.PublicKey) (solana.PublicKey, uint8, error) {
	indexBytes := make([]byte, 2)
	indexBytes[0] = byte(index)
	indexBytes[1] = byte(index >> 8)

	return solana.FindProgramAddress(
		[][]byte{
			[]byte("pool"),
			indexBytes,
			creator.Bytes(),
			cfg.BaseMint.Bytes(),
			cfg.QuoteMint.Bytes(),
		},
		cfg.ProgramID,
	)
}

// findAndValidatePool ищет пул для эффективной пары (baseMint, quoteMint) и проверяет, что
// найденный пул соответствует ожидаемым значениям (base mint должен совпадать).
func (dex *DEX) findAndValidatePool(ctx context.Context) (*PoolInfo, bool, error) {
	// Получаем эффективные значения минтов для свапа.
	effBase, effQuote := dex.effectiveMints()

	// Ищем пул с заданной парой с повторами.
	pool, err := dex.poolManager.FindPoolWithRetry(ctx, effBase, effQuote, 5, 2*time.Second)
	if err != nil {
		return nil, false, fmt.Errorf("failed to find pool: %w", err)
	}

	// Обновляем конфигурацию (адрес пула и LP-токена).
	dex.config.PoolAddress = pool.Address
	dex.config.LPMint = pool.LPMint

	dex.logger.Debug("Found pool details",
		zap.String("pool_address", pool.Address.String()),
		zap.String("base_mint", pool.BaseMint.String()),
		zap.String("quote_mint", pool.QuoteMint.String()))

	// Если пул найден в обратном порядке, вернём флаг poolMintReversed = true.
	poolMintReversed := false
	if !pool.BaseMint.Equals(effBase) {
		poolMintReversed = true
	}

	return pool, poolMintReversed, nil
}

// ParsePool парсит данные аккаунта в структуру Pool.
func ParsePool(data []byte) (*Pool, error) {
	if len(data) < 8 {
		return nil, fmt.Errorf("data too short for Pool")
	}

	for i := 0; i < 8; i++ {
		if data[i] != PoolDiscriminator[i] {
			return nil, fmt.Errorf("invalid discriminator for Pool")
		}
	}

	pos := 8

	if len(data) < pos+1+2+32+32+32+32+32+32+8 {
		return nil, fmt.Errorf("data too short for Pool content")
	}

	pool := &Pool{}

	pool.PoolBump = data[pos]
	pos++

	pool.Index = uint16(data[pos]) | (uint16(data[pos+1]) << 8)
	pos += 2

	creatorBytes := make([]byte, 32)
	copy(creatorBytes, data[pos:pos+32])
	pool.Creator = solana.PublicKeyFromBytes(creatorBytes)
	pos += 32

	baseMintBytes := make([]byte, 32)
	copy(baseMintBytes, data[pos:pos+32])
	pool.BaseMint = solana.PublicKeyFromBytes(baseMintBytes)
	pos += 32

	quoteMintBytes := make([]byte, 32)
	copy(quoteMintBytes, data[pos:pos+32])
	pool.QuoteMint = solana.PublicKeyFromBytes(quoteMintBytes)
	pos += 32

	lpMintBytes := make([]byte, 32)
	copy(lpMintBytes, data[pos:pos+32])
	pool.LPMint = solana.PublicKeyFromBytes(lpMintBytes)
	pos += 32

	poolBaseTokenAccountBytes := make([]byte, 32)
	copy(poolBaseTokenAccountBytes, data[pos:pos+32])
	pool.PoolBaseTokenAccount = solana.PublicKeyFromBytes(poolBaseTokenAccountBytes)
	pos += 32

	poolQuoteTokenAccountBytes := make([]byte, 32)
	copy(poolQuoteTokenAccountBytes, data[pos:pos+32])
	pool.PoolQuoteTokenAccount = solana.PublicKeyFromBytes(poolQuoteTokenAccountBytes)
	pos += 32

	pool.LPSupply = binary.LittleEndian.Uint64(data[pos : pos+8])

	return pool, nil
}
