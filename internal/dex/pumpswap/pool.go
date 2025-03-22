// =============================
// File: internal/dex/pumpswap/pool.go
// =============================

package pumpswap

import (
	"context"
	"encoding/binary"
	"fmt"
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
	mutex sync.RWMutex
	pools map[string]*PoolInfo // ключ: baseMint:quoteMint
}

// NewPoolCache создает новый кэш пулов
func NewPoolCache() *PoolCache {
	return &PoolCache{
		pools: make(map[string]*PoolInfo),
	}
}

// Get получает пул из кэша
func (pc *PoolCache) Get(baseMint, quoteMint solana.PublicKey) (*PoolInfo, bool) {
	pc.mutex.RLock()
	defer pc.mutex.RUnlock()

	key := makePoolCacheKey(baseMint, quoteMint)
	pool, exists := pc.pools[key]
	return pool, exists
}

// Set добавляет пул в кэш
func (pc *PoolCache) Set(baseMint, quoteMint solana.PublicKey, pool *PoolInfo) {
	pc.mutex.Lock()
	defer pc.mutex.Unlock()

	key := makePoolCacheKey(baseMint, quoteMint)
	pc.pools[key] = pool
}

// makePoolCacheKey создает ключ для кэша пулов
func makePoolCacheKey(baseMint, quoteMint solana.PublicKey) string {
	return fmt.Sprintf("%s:%s", baseMint.String(), quoteMint.String())
}

// PoolManager handles operations with PumpSwap pools
type PoolManager struct {
	client *solbc.Client
	logger *zap.Logger
	cache  *PoolCache
}

// NewPoolManager creates a new pool manager
func NewPoolManager(client *solbc.Client, logger *zap.Logger) *PoolManager {
	return &PoolManager{
		client: client,
		logger: logger.Named("pool_manager"),
		cache:  NewPoolCache(),
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

// FindPool finds a pool for the given token pair using deterministic discovery
func (pm *PoolManager) FindPool(
	ctx context.Context,
	baseMint, quoteMint solana.PublicKey,
) (*PoolInfo, error) {
	// Сначала проверяем кэш
	if pool, found := pm.cache.Get(baseMint, quoteMint); found {
		pm.logger.Debug("Found pool in cache",
			zap.String("base_mint", baseMint.String()),
			zap.String("quote_mint", quoteMint.String()))
		return pool, nil
	}

	// Попробуем найти через GetProgramAccounts (более эффективный метод)
	pool, err := pm.GetPoolByProgramAccounts(ctx, baseMint, quoteMint)
	if err == nil {
		return pool, nil
	}

	// Резервный метод - детерминистический поиск
	pm.logger.Debug("Falling back to deterministic pool discovery",
		zap.String("base_mint", baseMint.String()),
		zap.String("quote_mint", quoteMint.String()))

	// Создаем временную конфигурацию для получения адреса пула
	cfg := &Config{
		ProgramID: PumpSwapProgramID,
		BaseMint:  baseMint,
		QuoteMint: quoteMint,
	}

	// Расширенный список возможных создателей
	possibleCreators := []solana.PublicKey{
		PumpSwapProgramID, // Самый вероятный создатель - сама программа
		// Можно добавить других известных создателей пулов:
		solana.MustPublicKeyFromBase58("8LWu7QM2dGR1G8nKDHthckea57bkCzXyBTAKPJUBDHo8"), // Admin из IDL
	}

	// Создаем каналы для параллельного поиска
	type poolResult struct {
		pool *PoolInfo
		err  error
	}

	resultCh := make(chan poolResult, 1)

	// Функция для параллельного поиска пулов
	checkPool := func(index uint16, creator solana.PublicKey) {
		// Получаем адрес пула
		poolAddr, _, err := cfg.DerivePoolAddress(index, creator)
		if err != nil {
			return // Просто пропускаем
		}

		// Проверяем существование аккаунта
		accountInfo, err := pm.client.GetAccountInfo(ctx, poolAddr)
		// Игнорируем "not found" ошибки, это ожидаемо при поиске
		if err != nil || accountInfo == nil || accountInfo.Value == nil {
			return
		}

		// Проверяем, владелец - программа PumpSwap
		if !accountInfo.Value.Owner.Equals(PumpSwapProgramID) {
			return
		}

		// Пробуем распарсить данные пула
		poolData := accountInfo.Value.Data.GetBinary()
		pool, err := ParsePool(poolData)
		if err != nil {
			pm.logger.Debug("Failed to parse pool data",
				zap.String("pool_address", poolAddr.String()),
				zap.Error(err))
			return
		}

		// Проверяем пару токенов
		if !pool.BaseMint.Equals(baseMint) || !pool.QuoteMint.Equals(quoteMint) {
			return
		}

		// Нашли подходящий пул, получаем полную информацию
		poolInfo, err := pm.FetchPoolInfo(ctx, poolAddr)
		if err != nil {
			pm.logger.Error("Failed to fetch pool info",
				zap.String("pool_address", poolAddr.String()),
				zap.Error(err))
			return
		}

		// Отправляем результат в канал
		select {
		case resultCh <- poolResult{pool: poolInfo, err: nil}:
			// Успешно отправили
		case <-ctx.Done():
			// Контекст завершен
		}
	}

	// Запускаем воркеры с ограничением concurrency
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 10) // Макс 10 параллельных запросов

	// Проверяем первые 10 индексов для всех создателей (наиболее вероятно найти пул там)
	for index := uint16(0); index < 10 && ctx.Err() == nil; index++ {
		for _, creator := range possibleCreators {
			select {
			case <-ctx.Done():
				break
			case semaphore <- struct{}{}:
				wg.Add(1)
				go func(idx uint16, cr solana.PublicKey) {
					defer wg.Done()
					defer func() { <-semaphore }()
					checkPool(idx, cr)
				}(index, creator)
			}
		}
	}

	// Если не нашли в первых 10, проверяем остальные
	if ctx.Err() == nil {
		for index := uint16(10); index < 100 && ctx.Err() == nil; index++ {
			for _, creator := range possibleCreators {
				select {
				case <-ctx.Done():
					break
				case semaphore <- struct{}{}:
					wg.Add(1)
					go func(idx uint16, cr solana.PublicKey) {
						defer wg.Done()
						defer func() { <-semaphore }()
						checkPool(idx, cr)
					}(index, creator)
				}
			}
		}
	}

	// Закрываем канал результатов после завершения всех воркеров
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// Получаем первый успешный результат
	select {
	case result := <-resultCh:
		if result.err == nil && result.pool != nil {
			// Сохраняем в кэш
			pm.cache.Set(baseMint, quoteMint, result.pool)
			return result.pool, nil
		}
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	return nil, fmt.Errorf("no pool found for base mint %s and quote mint %s",
		baseMint.String(), quoteMint.String())
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
		PumpSwapProgramID,
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

	// Get token accounts data to check reserves
	baseTokenInfo, err := pm.client.GetAccountInfo(ctx, pool.PoolBaseTokenAccount)
	if err != nil {
		return nil, fmt.Errorf("failed to get base token account: %w", err)
	}

	quoteTokenInfo, err := pm.client.GetAccountInfo(ctx, pool.PoolQuoteTokenAccount)
	if err != nil {
		return nil, fmt.Errorf("failed to get quote token account: %w", err)
	}

	// Parse token accounts to get reserves
	var baseReserves, quoteReserves uint64

	if baseTokenInfo != nil && baseTokenInfo.Value != nil {
		// Extract token balance from the account data
		// SPL Token account data structure:
		// - Mint: 32 bytes (offset 0)
		// - Owner: 32 bytes (offset 32)
		// - Amount: 8 bytes (offset 64)
		baseData := baseTokenInfo.Value.Data.GetBinary()
		if len(baseData) >= 72 {
			// SPL Token amounts are stored in little-endian format
			baseReserves = binary.LittleEndian.Uint64(baseData[64:72])
		}
	}

	if quoteTokenInfo != nil && quoteTokenInfo.Value != nil {
		quoteData := quoteTokenInfo.Value.Data.GetBinary()
		if len(quoteData) >= 72 {
			// SPL Token amounts are stored in little-endian format
			quoteReserves = binary.LittleEndian.Uint64(quoteData[64:72])
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
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		// Создаем контекст с таймаутом для каждой попытки
		searchCtx, cancel := context.WithTimeout(ctx, time.Second*5)

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
