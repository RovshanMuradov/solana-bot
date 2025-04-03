// =============================
// File: internal/dex/pumpswap/pool.go
// =============================
package pumpswap

import (
	"context"
	"encoding/binary"
	"fmt"
	"github.com/cenkalti/backoff/v5"
	"math"
	"sync"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"go.uber.org/zap"

	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc"
)

const (
	MinimumLiquidity         uint64 = 1000
	TokenAccountMintOffset   uint64 = 0
	TokenAccountOwnerOffset  uint64 = 32
	TokenAccountAmountOffset uint64 = 64
	TokenAccountAmountSize   uint64 = 8
)

type PoolManagerInterface interface {
	FindPool(ctx context.Context, baseMint, quoteMint solana.PublicKey) (*PoolInfo, error)
	FindPoolWithRetry(ctx context.Context, baseMint, quoteMint solana.PublicKey, maxRetries int, retryDelay time.Duration) (*PoolInfo, error)
	CalculateSwapQuote(pool *PoolInfo, inputAmount uint64, isBaseToQuote bool) (uint64, float64)
	CalculateSlippage(pool *PoolInfo, inputAmount uint64, isBaseToQuote bool) float64
	FetchPoolInfo(ctx context.Context, poolAddress solana.PublicKey) (*PoolInfo, error)
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

// PoolCache stores found pools for quick access
type PoolCache struct {
	mutex      sync.RWMutex
	pools      map[string]*PoolInfo // key: hashed pool key
	expiration map[string]time.Time
	ttl        time.Duration
}

// NewPoolCache создает новый кэш пулов с указанным временем жизни записей.
//
// Метод инициализирует новый экземпляр PoolCache с указанным TTL (time-to-live)
// для записей. Если TTL установлен в 0, используется значение по умолчанию
// в 5 минут. Кэш используется для хранения информации о пулах, чтобы избежать
// повторных запросов к блокчейну.
//
// Параметры:
//   - ttl: время жизни записей в кэше (duration)
//
// Возвращает:
//   - *PoolCache: указатель на новый объект кэша пулов
func NewPoolCache(ttl time.Duration) *PoolCache {
	if ttl == 0 {
		ttl = 5 * time.Minute
	}

	return &PoolCache{
		pools:      make(map[string]*PoolInfo),
		expiration: make(map[string]time.Time),
		ttl:        ttl,
	}
}

// Get извлекает информацию о пуле из кэша, проверяя срок действия.
//
// Метод проверяет наличие пула по паре токенов (базовый и котировочный) в кэше
// и возвращает его, если срок действия кэша не истек. Если пул не найден или
// срок действия истек, возвращается false во втором возвращаемом значении.
//
// Параметры:
//   - baseMint: публичный ключ базового токена
//   - quoteMint: публичный ключ котировочного токена
//
// Возвращает:
//   - *PoolInfo: информация о пуле или nil, если пул не найден
//   - bool: true, если пул найден и актуален, иначе false
func (pc *PoolCache) Get(baseMint, quoteMint solana.PublicKey) (*PoolInfo, bool) {
	pc.mutex.RLock()
	defer pc.mutex.RUnlock()

	key := makePoolCacheKey(baseMint, quoteMint)
	pool, exists := pc.pools[key]

	if !exists {
		return nil, false
	}

	expiry, hasExpiry := pc.expiration[key]
	if hasExpiry && time.Now().After(expiry) {
		return nil, false
	}

	return pool, true
}

// Set добавляет информацию о пуле в кэш с установкой времени истечения.
//
// Метод сначала очищает устаревшие записи в кэше, затем добавляет новую запись
// с информацией о пуле. Время истечения рассчитывается как текущее время плюс TTL.
//
// Параметры:
//   - baseMint: публичный ключ базового токена
//   - quoteMint: публичный ключ котировочного токена
//   - pool: указатель на структуру с информацией о пуле
func (pc *PoolCache) Set(baseMint, quoteMint solana.PublicKey, pool *PoolInfo) {
	pc.mutex.Lock()
	defer pc.mutex.Unlock()

	pc.cleanExpired()

	key := makePoolCacheKey(baseMint, quoteMint)
	pc.pools[key] = pool
	pc.expiration[key] = time.Now().Add(pc.ttl)
}

// cleanExpired удаляет устаревшие записи из кэша.
//
// Метод проверяет все записи в кэше и удаляет те, у которых
// истекло время действия (текущее время превышает время истечения).
// Это позволяет освободить память и поддерживать кэш актуальным.
func (pc *PoolCache) cleanExpired() {
	now := time.Now()
	for key, expiry := range pc.expiration {
		if now.After(expiry) {
			delete(pc.pools, key)
			delete(pc.expiration, key)
		}
	}
}

// makePoolCacheKey создает уникальный ключ для идентификации пула в кэше.
//
// Метод формирует строковый ключ на основе публичных ключей базового и котировочного
// токенов. Ключи токенов сортируются для обеспечения одинакового результата
// независимо от порядка передачи аргументов.
//
// Параметры:
//   - baseMint: публичный ключ базового токена
//   - quoteMint: публичный ключ котировочного токена
//
// Возвращает:
//   - string: строковый ключ для идентификации пула в кэше
func makePoolCacheKey(baseMint, quoteMint solana.PublicKey) string {
	// Sort mints for consistent key regardless of order
	if baseMint.String() < quoteMint.String() {
		return fmt.Sprintf("%s:%s", baseMint, quoteMint)
	}
	return fmt.Sprintf("%s:%s", quoteMint, baseMint)
}

// DefaultPoolManagerOptions возвращает настройки по умолчанию для менеджера пулов.
//
// Функция создает и возвращает структуру PoolManagerOptions с предустановленными
// значениями по умолчанию для TTL кэша, максимальных повторов, задержки между
// повторами и ID программы PumpSwap.
//
// Возвращает:
//   - PoolManagerOptions: структура с настройками по умолчанию
func DefaultPoolManagerOptions() PoolManagerOptions {
	return PoolManagerOptions{
		CacheTTL:   5 * time.Minute,
		MaxRetries: 3,
		RetryDelay: time.Second,
		ProgramID:  PumpSwapProgramID,
	}
}

// NewPoolManager создает новый менеджер пулов с указанными опциями.
//
// Метод инициализирует новый экземпляр PoolManager с переданным клиентом Solana,
// логгером и опциями. Если опции не указаны, используются значения по умолчанию.
// PoolManager отвечает за поиск, кэширование и взаимодействие с пулами PumpSwap.
//
// Параметры:
//   - client: клиент для взаимодействия с блокчейном Solana
//   - logger: логгер для записи сообщений
//   - opts: вариативный параметр с опциями для менеджера пулов
//
// Возвращает:
//   - *PoolManager: указатель на новый объект менеджера пулов
func NewPoolManager(client *solbc.Client, logger *zap.Logger, opts ...PoolManagerOptions) *PoolManager {
	defaultOpts := DefaultPoolManagerOptions()

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

// FindPool находит пул для заданной пары токенов.
//
// Метод ищет пул для указанной пары токенов сначала в кэше (в прямом и обратном
// порядке токенов), а затем, если не найден, выполняет поиск в блокчейне. Найденный
// пул добавляется в кэш для последующего использования.
//
// Параметры:
//   - ctx: контекст выполнения операции
//   - baseMint: публичный ключ базового токена
//   - quoteMint: публичный ключ котировочного токена
//
// Возвращает:
//   - *PoolInfo: информация о найденном пуле
//   - error: ошибка, если пул не найден или произошла ошибка при поиске
func (pm *PoolManager) FindPool(
	ctx context.Context,
	baseMint, quoteMint solana.PublicKey,
) (*PoolInfo, error) {
	if pool, found := pm.cache.Get(baseMint, quoteMint); found {
		pm.logger.Debug("Found pool in cache",
			zap.String("base_mint", baseMint.String()),
			zap.String("quote_mint", quoteMint.String()))
		return pool, nil
	}

	if pool, found := pm.cache.Get(quoteMint, baseMint); found {
		pm.logger.Debug("Found pool in cache (reversed order)",
			zap.String("base_mint", quoteMint.String()),
			zap.String("quote_mint", baseMint.String()))

		pool.BaseMint, pool.QuoteMint = pool.QuoteMint, pool.BaseMint
		pool.BaseReserves, pool.QuoteReserves = pool.QuoteReserves, pool.BaseReserves
		pool.PoolBaseTokenAccount, pool.PoolQuoteTokenAccount = pool.PoolQuoteTokenAccount, pool.PoolBaseTokenAccount
		return pool, nil
	}

	pool, err := pm.findPoolByProgramAccounts(ctx, baseMint, quoteMint)
	if err == nil && pool != nil {
		pm.cache.Set(baseMint, quoteMint, pool)
		return pool, nil
	}

	pool, err = pm.findPoolByProgramAccounts(ctx, quoteMint, baseMint)
	if err == nil && pool != nil {
		pool.BaseMint, pool.QuoteMint = pool.QuoteMint, pool.BaseMint
		pool.BaseReserves, pool.QuoteReserves = pool.QuoteReserves, pool.BaseReserves
		pool.PoolBaseTokenAccount, pool.PoolQuoteTokenAccount = pool.PoolQuoteTokenAccount, pool.PoolBaseTokenAccount

		pm.cache.Set(baseMint, quoteMint, pool)
		return pool, nil
	}

	return nil, fmt.Errorf("no pool found for base mint %s and quote mint %s",
		baseMint.String(), quoteMint.String())
}

// findPoolByProgramAccounts ищет пул через сканирование программных аккаунтов.
//
// Метод выполняет запрос к программным аккаунтам PumpSwap с фильтрацией по
// дискриминатору пула и базовому токену. Для найденных аккаунтов проверяется
// соответствие парам токенов и извлекается полная информация о пуле.
//
// Параметры:
//   - ctx: контекст выполнения операции
//   - baseMint: публичный ключ базового токена
//   - quoteMint: публичный ключ котировочного токена
//
// Возвращает:
//   - *PoolInfo: информация о найденном пуле или nil, если пул не найден
//   - error: ошибка, если произошла ошибка при поиске
func (pm *PoolManager) findPoolByProgramAccounts(
	ctx context.Context,
	baseMint, quoteMint solana.PublicKey,
) (*PoolInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

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
			{
				Memcmp: &rpc.RPCFilterMemcmp{
					Offset: 8 + 1 + 2 + 32,
					Bytes:  baseMint.Bytes(),
				},
			},
		},
	}

	accounts, err := pm.client.GetProgramAccountsWithOpts(ctx, pm.programID, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to get program accounts: %w", err)
	}

	for _, account := range accounts {
		poolData := account.Account.Data.GetBinary()
		pool, err := ParsePool(poolData)
		if err != nil {
			continue
		}

		// Check if pool matches our token pair
		isMatch := (pool.BaseMint.Equals(baseMint) && pool.QuoteMint.Equals(quoteMint)) ||
			(pool.BaseMint.Equals(quoteMint) && pool.QuoteMint.Equals(baseMint))

		if !isMatch {
			continue
		}

		poolInfo, err := pm.FetchPoolInfo(ctx, account.Pubkey)
		if err != nil {
			continue
		}

		if poolInfo.BaseReserves == 0 || poolInfo.QuoteReserves == 0 {
			continue
		}

		pm.logger.Info("Found PumpSwap pool",
			zap.String("pool_address", account.Pubkey.String()),
			zap.String("base_mint", pool.BaseMint.String()),
			zap.String("quote_mint", pool.QuoteMint.String()))

		return poolInfo, nil
	}

	return nil, fmt.Errorf("no matching pool found for %s/%s",
		baseMint.String(), quoteMint.String())
}

// fetchGlobalConfig получает глобальную конфигурацию программы PumpSwap.
//
// Метод вычисляет адрес глобальной конфигурации через PDA (Program Derived Address),
// получает данные аккаунта и парсит их в структуру GlobalConfig. Эта конфигурация
// содержит общие настройки для всех пулов, такие как комиссии.
//
// Параметры:
//   - ctx: контекст выполнения операции
//
// Возвращает:
//   - *GlobalConfig: указатель на структуру с глобальной конфигурацией
//   - error: ошибка, если не удалось получить или распарсить конфигурацию
func (pm *PoolManager) fetchGlobalConfig(ctx context.Context) (*GlobalConfig, error) {
	globalConfig, _, err := solana.FindProgramAddress(
		[][]byte{[]byte("global_config")},
		pm.programID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to derive global config address: %w", err)
	}
	accountInfo, err := pm.client.GetAccountInfo(ctx, globalConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to get global config account: %w", err)
	}
	if accountInfo == nil || accountInfo.Value == nil {
		return nil, fmt.Errorf("global config account not found: %s", globalConfig.String())
	}
	globalData := accountInfo.Value.Data.GetBinary()
	config, err := ParseGlobalConfig(globalData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse global config: %w", err)
	}
	return config, nil
}

// parseTokenAccounts извлекает балансы из бинарных данных токен-аккаунтов.
//
// Метод парсит байтовые представления токен-аккаунтов SPL и извлекает из них
// поля с балансами (reserves). Используется для определения текущей ликвидности
// в пуле.
//
// Параметры:
//   - baseData: бинарные данные аккаунта базового токена
//   - quoteData: бинарные данные аккаунта котировочного токена
//
// Возвращает:
//   - uint64: баланс базового токена (количество единиц)
//   - uint64: баланс котировочного токена (количество единиц)
func parseTokenAccounts(baseData, quoteData []byte) (uint64, uint64) {
	var baseReserves, quoteReserves uint64
	if len(baseData) >= int(TokenAccountAmountOffset+TokenAccountAmountSize) {
		baseReserves = binary.LittleEndian.Uint64(baseData[TokenAccountAmountOffset : TokenAccountAmountOffset+TokenAccountAmountSize])
	}
	if len(quoteData) >= int(TokenAccountAmountOffset+TokenAccountAmountSize) {
		quoteReserves = binary.LittleEndian.Uint64(quoteData[TokenAccountAmountOffset : TokenAccountAmountOffset+TokenAccountAmountSize])
	}
	return baseReserves, quoteReserves
}

// getPool извлекает и парсит данные пула по адресу.
//
// Метод получает информацию об аккаунте пула из блокчейна по указанному адресу
// и парсит бинарные данные в структуру Pool.
//
// Параметры:
//   - ctx: контекст выполнения операции
//   - poolAddress: публичный ключ аккаунта пула
//
// Возвращает:
//   - *Pool: указатель на структуру с данными пула
//   - error: ошибка, если не удалось получить или распарсить данные пула
func (pm *PoolManager) getPool(ctx context.Context, poolAddress solana.PublicKey) (*Pool, error) {
	accountInfo, err := pm.client.GetAccountInfo(ctx, poolAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to get pool account: %w", err)
	}
	if accountInfo == nil || accountInfo.Value == nil {
		return nil, fmt.Errorf("pool account not found: %s", poolAddress.String())
	}
	return ParsePool(accountInfo.Value.Data.GetBinary())
}

// getTokenAccountsData получает бинарные данные для заданных аккаунтов.
//
// Метод выполняет пакетный запрос к блокчейну для получения информации
// о нескольких аккаунтах одновременно и извлекает из них бинарные данные.
//
// Параметры:
//   - ctx: контекст выполнения операции
//   - accounts: массив публичных ключей аккаунтов для запроса
//
// Возвращает:
//   - [][]byte: массив бинарных данных для каждого запрошенного аккаунта
//   - error: ошибка, если не удалось получить данные аккаунтов
func (pm *PoolManager) getTokenAccountsData(
	ctx context.Context, accounts []solana.PublicKey,
) ([][]byte, error) {
	accountsInfo, err := pm.client.GetMultipleAccounts(ctx, accounts)
	if err != nil {
		return nil, fmt.Errorf("failed to get accounts info: %w", err)
	}
	if accountsInfo == nil || accountsInfo.Value == nil || len(accountsInfo.Value) < len(accounts) {
		return nil, fmt.Errorf("failed to get required token accounts")
	}

	data := make([][]byte, len(accounts))
	for i, info := range accountsInfo.Value {
		if info != nil {
			data[i] = info.Data.GetBinary()
		}
	}
	return data, nil
}

// FetchPoolInfo получает полную информацию о пуле по его адресу.
//
// Метод извлекает данные пула, глобальную конфигурацию и балансы токен-аккаунтов
// пула, объединяя их в структуру PoolInfo. Эта структура содержит всю необходимую
// информацию для работы с пулом, включая адреса, балансы и комиссии.
//
// Параметры:
//   - ctx: контекст выполнения операции
//   - poolAddress: публичный ключ аккаунта пула
//
// Возвращает:
//   - *PoolInfo: указатель на структуру с полной информацией о пуле
//   - error: ошибка, если не удалось получить или заполнить информацию о пуле
func (pm *PoolManager) FetchPoolInfo(
	ctx context.Context,
	poolAddress solana.PublicKey,
) (*PoolInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	pool, err := pm.getPool(ctx, poolAddress)
	if err != nil {
		return nil, err
	}

	config, err := pm.fetchGlobalConfig(ctx)
	if err != nil {
		return nil, err
	}

	accounts := []solana.PublicKey{
		pool.PoolBaseTokenAccount,
		pool.PoolQuoteTokenAccount,
	}

	accountsData, err := pm.getTokenAccountsData(ctx, accounts)
	if err != nil {
		return nil, err
	}

	var baseData, quoteData []byte
	if len(accountsData) > 0 {
		baseData = accountsData[0]
	}
	if len(accountsData) > 1 {
		quoteData = accountsData[1]
	}
	baseReserves, quoteReserves := parseTokenAccounts(baseData, quoteData)

	return &PoolInfo{
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
	}, nil
}

// CalculateSwapQuote рассчитывает ожидаемый результат обмена в пуле.
//
// Метод вычисляет, сколько токенов пользователь получит при обмене указанного
// количества входящего токена, учитывая текущие резервы пула и комиссию.
// Также рассчитывается текущий курс обмена для этой операции.
//
// Параметры:
//   - pool: информация о пуле для расчета
//   - inputAmount: количество входящего токена для обмена (в минимальных единицах)
//   - isBaseToQuote: направление обмена (true - из базового в котировочный,
//     false - из котировочного в базовый)
//
// Возвращает:
//   - uint64: ожидаемое количество исходящего токена (в минимальных единицах)
//   - float64: курс обмена (отношение выходного количества к входному)
func (pm *PoolManager) CalculateSwapQuote(pool *PoolInfo, inputAmount uint64, isBaseToQuote bool) (uint64, float64) {
	feeFactor := 1.0 - (float64(pool.FeesBasisPoints) / 10000.0)

	if isBaseToQuote {
		output := calculateOutput(pool.BaseReserves, pool.QuoteReserves, inputAmount, feeFactor)
		price := float64(0)
		if inputAmount > 0 {
			price = float64(output) / float64(inputAmount)
		}
		return output, price
	}

	output := calculateOutput(pool.QuoteReserves, pool.BaseReserves, inputAmount, feeFactor)
	price := float64(0)
	if output > 0 {
		price = float64(inputAmount) / float64(output)
	}
	return output, price
}

// CalculateSlippage рассчитывает проскальзывание цены при свопе указанного объема.
//
// Метод определяет разницу между текущей ценой в пуле и фактической ценой,
// по которой будет выполнен своп указанного объема токена. Проскальзывание
// выражается в процентах и показывает, насколько цена изменится из-за
// влияния размера операции на ликвидность пула.
//
// Параметры:
//   - pool: информация о пуле для расчета
//   - inputAmount: количество входящего токена для обмена (в минимальных единицах)
//   - isBaseToQuote: направление обмена (true - из базового в котировочный,
//     false - из котировочного в базовый)
//
// Возвращает:
//   - float64: проскальзывание в процентах
func (pm *PoolManager) CalculateSlippage(
	pool *PoolInfo,
	inputAmount uint64,
	isBaseToQuote bool,
) float64 {
	var initialPrice, finalPrice float64

	if isBaseToQuote {
		initialPrice = float64(pool.QuoteReserves) / float64(pool.BaseReserves)
		outputAmount, _ := pm.CalculateSwapQuote(pool, inputAmount, true)
		finalPrice = float64(pool.QuoteReserves-outputAmount) /
			float64(pool.BaseReserves+inputAmount)
	} else {
		initialPrice = float64(pool.BaseReserves) / float64(pool.QuoteReserves)
		outputAmount, _ := pm.CalculateSwapQuote(pool, inputAmount, false)
		finalPrice = float64(pool.BaseReserves-outputAmount) /
			float64(pool.QuoteReserves+inputAmount)
	}

	slippage := math.Abs(finalPrice-initialPrice) / initialPrice * 100

	return slippage
}

// FindPoolWithRetry ищет пул для пары токенов с повторными попытками.
//
// Метод выполняет поиск пула с автоматическими повторами в случае неудачи.
// Используется экспоненциальная стратегия задержки между попытками.
// Этот метод полезен при нестабильном соединении или высокой нагрузке на RPC.
//
// Параметры:
//   - ctx: контекст выполнения операции
//   - baseMint: публичный ключ базового токена
//   - quoteMint: публичный ключ котировочного токена
//   - maxRetries: максимальное количество повторных попыток (0 - использовать значение из менеджера)
//   - retryDelay: начальная задержка между попытками (0 - использовать значение из менеджера)
//
// Возвращает:
//   - *PoolInfo: информация о найденном пуле
//   - error: ошибка, если пул не найден после всех попыток
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

	backoffPolicy := backoff.NewExponentialBackOff()
	backoffPolicy.InitialInterval = retryDelay
	backoffPolicy.MaxInterval = retryDelay * 10

	// Create a properly typed operation function
	operation := func() (*PoolInfo, error) {
		pool, err := pm.FindPool(ctx, baseMint, quoteMint)
		if err != nil {
			pm.logger.Debug("Failed to find pool, retrying",
				zap.String("base", baseMint.String()),
				zap.String("quote", quoteMint.String()),
				zap.Error(err))
			return nil, err
		}
		return pool, nil
	}

	// Create a notify function
	notify := func(err error, duration time.Duration) {
		pm.logger.Debug("Retry after error",
			zap.Error(err),
			zap.Duration("backoff", duration))
	}

	// Use the proper option functions
	var maxTriesUint uint = 1
	if maxRetries > 0 {
		maxTriesUint = uint(maxRetries)
	}

	// Use the Retry function with correct options
	return backoff.Retry(
		ctx,
		operation,
		backoff.WithBackOff(backoffPolicy),
		backoff.WithMaxTries(maxTriesUint),
		backoff.WithNotify(notify),
	)
}

// DerivePoolAddress вычисляет адрес пула на основе параметров.
//
// Метод использует алгоритм Program Derived Address (PDA) для получения
// детерминированного адреса пула на основе индекса, создателя и пары токенов.
// Этот адрес используется для взаимодействия с пулом и проверки его существования.
//
// Параметры:
//   - index: индекс пула
//   - creator: публичный ключ создателя пула
//
// Возвращает:
//   - solana.PublicKey: публичный ключ адреса пула
//   - uint8: bump значение для PDA
//   - error: ошибка, если не удалось вычислить адрес
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

// findAndValidatePool находит и проверяет пул для текущей конфигурации DEX.
//
// Метод ищет пул для токенов, указанных в конфигурации DEX, проверяет его
// валидность и обновляет конфигурацию адресом найденного пула. Также
// определяет, перевернут ли порядок токенов в пуле относительно конфигурации.
//
// Параметры:
//   - ctx: контекст выполнения операции
//
// Возвращает:
//   - *PoolInfo: информация о найденном пуле
//   - bool: флаг, указывающий, перевернут ли порядок токенов в пуле
//   - error: ошибка, если пул не найден или не валиден
func (dex *DEX) findAndValidatePool(ctx context.Context) (*PoolInfo, bool, error) {
	effBase, effQuote := dex.effectiveMints()

	pool, err := dex.poolManager.FindPoolWithRetry(ctx, effBase, effQuote, 5, 2*time.Second)
	if err != nil {
		return nil, false, fmt.Errorf("failed to find pool: %w", err)
	}

	dex.config.PoolAddress = pool.Address
	dex.config.LPMint = pool.LPMint

	dex.logger.Debug("Found pool details",
		zap.String("pool_address", pool.Address.String()),
		zap.String("base_mint", pool.BaseMint.String()),
		zap.String("quote_mint", pool.QuoteMint.String()))

	poolMintReversed := !pool.BaseMint.Equals(effBase)

	return pool, poolMintReversed, nil
}

// ParsePool парсит бинарные данные аккаунта пула.
//
// Метод проверяет корректность дискриминатора пула и извлекает все поля
// из бинарного представления аккаунта в структуру Pool. Он обрабатывает
// сырые байты, полученные из блокчейна, в структурированные данные.
//
// Параметры:
//   - data: бинарные данные аккаунта пула
//
// Возвращает:
//   - *Pool: указатель на структуру с данными пула
//   - error: ошибка, если данные некорректны или не соответствуют формату пула
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

	pool.Creator = solana.PublicKeyFromBytes(data[pos : pos+32])
	pos += 32

	pool.BaseMint = solana.PublicKeyFromBytes(data[pos : pos+32])
	pos += 32

	pool.QuoteMint = solana.PublicKeyFromBytes(data[pos : pos+32])
	pos += 32

	pool.LPMint = solana.PublicKeyFromBytes(data[pos : pos+32])
	pos += 32

	pool.PoolBaseTokenAccount = solana.PublicKeyFromBytes(data[pos : pos+32])
	pos += 32

	pool.PoolQuoteTokenAccount = solana.PublicKeyFromBytes(data[pos : pos+32])
	pos += 32

	pool.LPSupply = binary.LittleEndian.Uint64(data[pos : pos+8])

	return pool, nil
}
