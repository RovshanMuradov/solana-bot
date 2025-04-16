// =============================
// File: internal/dex/pumpswap/pool.go
// =============================
package pumpswap

import (
	"context"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/cenkalti/backoff/v5"
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

////////////////////////////////////////////////////////////////////////////////
// Интерфейс и конструкторы
////////////////////////////////////////////////////////////////////////////////

// PoolManagerInterface определяет набор основных методов для работы с пулами.
type PoolManagerInterface interface {
	FindPool(ctx context.Context, baseMint, quoteMint solana.PublicKey) (*PoolInfo, error)
	FindPoolWithRetry(ctx context.Context, baseMint, quoteMint solana.PublicKey, maxRetries int, retryDelay time.Duration) (*PoolInfo, error)
	CalculateSwapQuote(pool *PoolInfo, inputAmount uint64, isBaseToQuote bool) (uint64, float64)
	FetchPoolInfo(ctx context.Context, poolAddress solana.PublicKey) (*PoolInfo, error)
}

// PoolManager отвечает за операции с пулами PumpSwap.
type PoolManager struct {
	client     *solbc.Client
	logger     *zap.Logger
	programID  solana.PublicKey
	maxRetries int
	retryDelay time.Duration
}

// PoolManagerOptions содержит опции для создания нового PoolManager.
type PoolManagerOptions struct {
	MaxRetries int
	RetryDelay time.Duration
	ProgramID  solana.PublicKey
}

// DefaultPoolManagerOptions возвращает настройки по умолчанию.
func DefaultPoolManagerOptions() PoolManagerOptions {
	return PoolManagerOptions{
		MaxRetries: 3,
		RetryDelay: time.Second,
		ProgramID:  PumpSwapProgramID,
	}
}

// NewPoolManager создаёт новый PoolManager с заданными опциями.
func NewPoolManager(client *solbc.Client, logger *zap.Logger, opts ...PoolManagerOptions) *PoolManager {
	var options PoolManagerOptions
	if len(opts) > 0 {
		options = opts[0]
	} else {
		options = DefaultPoolManagerOptions()
	}

	logger.Info("Создание нового PoolManager",
		zap.String("program_id", options.ProgramID.String()),
		zap.Int("max_retries", options.MaxRetries),
		zap.Duration("retry_delay", options.RetryDelay))

	return &PoolManager{
		client:     client,
		logger:     logger.Named("pool_manager"),
		programID:  options.ProgramID,
		maxRetries: options.MaxRetries,
		retryDelay: options.RetryDelay,
	}
}

////////////////////////////////////////////////////////////////////////////////
// Вспомогательные функции
////////////////////////////////////////////////////////////////////////////////

// getAccountBinaryData получает бинарные данные аккаунта по его publicKey.
func (pm *PoolManager) getAccountBinaryData(ctx context.Context, pubkey solana.PublicKey) ([]byte, error) {
	accountInfo, err := pm.client.GetAccountInfo(ctx, pubkey)
	if err != nil {
		pm.logger.Error("Не удалось получить информацию об аккаунте", zap.String("account", pubkey.String()), zap.Error(err))
		return nil, fmt.Errorf("failed to get account info for %s: %w", pubkey.String(), err)
	}
	if accountInfo == nil || accountInfo.Value == nil {
		pm.logger.Error("Аккаунт не найден", zap.String("account", pubkey.String()))
		return nil, fmt.Errorf("account not found: %s", pubkey.String())
	}
	return accountInfo.Value.Data.GetBinary(), nil
}

// getAccountBinaryDataMultiple получает бинарные данные для множества аккаунтов.
func (pm *PoolManager) getAccountBinaryDataMultiple(ctx context.Context, accounts []solana.PublicKey) ([][]byte, error) {
	accountsStr := make([]string, len(accounts))
	for i, acc := range accounts {
		accountsStr[i] = acc.String()
	}
	accountsInfo, err := pm.client.GetMultipleAccounts(ctx, accounts)
	if err != nil {
		pm.logger.Error("Не удалось получить информацию по нескольким аккаунтам", zap.Strings("accounts", accountsStr), zap.Error(err))
		return nil, fmt.Errorf("failed to get accounts info: %w", err)
	}
	if accountsInfo == nil || accountsInfo.Value == nil || len(accountsInfo.Value) < len(accounts) {
		pm.logger.Error("Получено недостаточное количество данных об аккаунтах", zap.Strings("accounts", accountsStr),
			zap.Int("requested", len(accounts)), zap.Int("received", len(accountsInfo.Value)))
		return nil, fmt.Errorf("failed to get required token accounts")
	}

	data := make([][]byte, len(accounts))
	for i, info := range accountsInfo.Value {
		if info != nil {
			data[i] = info.Data.GetBinary()
		} else {
			pm.logger.Warn("Данные токен-аккаунта равны nil", zap.String("account", accounts[i].String()))
		}
	}
	return data, nil
}

// parseTokenAccounts извлекает балансы из бинарных данных токен-аккаунтов.
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

////////////////////////////////////////////////////////////////////////////////
// Основные методы для работы с пулами
////////////////////////////////////////////////////////////////////////////////

// FindPool ищет пул для заданной пары токенов, сперва пытаясь найти в прямом порядке, затем – в обратном.
func (pm *PoolManager) FindPool(ctx context.Context, baseMint, quoteMint solana.PublicKey) (*PoolInfo, error) {

	// Пробуем прямой порядок
	poolInfo, err := pm.findPoolByProgramAccounts(ctx, baseMint, quoteMint)
	if err == nil && poolInfo != nil {
		return poolInfo, nil
	}

	pm.logger.Info("Пул в прямом порядке не найден, пробуем обратный порядок")

	// Пробуем обратный порядок
	poolInfo, err = pm.findPoolByProgramAccounts(ctx, quoteMint, baseMint)
	if err == nil && poolInfo != nil {
		pm.logger.Info("Найден пул в обратном порядке", zap.String("pool_address", poolInfo.Address.String()))
		// Меняем местами поля для корректного результата
		poolInfo.BaseMint, poolInfo.QuoteMint = poolInfo.QuoteMint, poolInfo.BaseMint
		poolInfo.BaseReserves, poolInfo.QuoteReserves = poolInfo.QuoteReserves, poolInfo.BaseReserves
		poolInfo.PoolBaseTokenAccount, poolInfo.PoolQuoteTokenAccount = poolInfo.PoolQuoteTokenAccount, poolInfo.PoolBaseTokenAccount
		return poolInfo, nil
	}

	pm.logger.Error("Пул не найден ни в прямом, ни в обратном порядке", zap.String("base_mint", baseMint.String()), zap.String("quote_mint", quoteMint.String()), zap.Error(err))
	return nil, fmt.Errorf("no pool found for base mint %s and quote mint %s", baseMint.String(), quoteMint.String())
}

// findPoolByProgramAccounts сканирует программные аккаунты по заданному фильтру и возвращает первый подходящий пул.
func (pm *PoolManager) findPoolByProgramAccounts(ctx context.Context, baseMint, quoteMint solana.PublicKey) (*PoolInfo, error) {

	// Ограничиваем время запроса
	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
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
					Offset: 8 + 1 + 2 + 32, // смещение, где хранится baseMint
					Bytes:  baseMint.Bytes(),
				},
			},
		},
	}

	accounts, err := pm.client.GetProgramAccountsWithOpts(timeoutCtx, pm.programID, opts)
	if err != nil {
		pm.logger.Error("Не удалось получить программные аккаунты", zap.Error(err))
		return nil, fmt.Errorf("failed to get program accounts: %w", err)
	}

	for _, account := range accounts {
		pool, err := ParsePool(account.Account.Data.GetBinary())
		if err != nil {
			continue
		}

		// Проверяем, соответствует ли пул искомой паре токенов (учитывая оба порядка)
		if !(pool.BaseMint.Equals(baseMint) && pool.QuoteMint.Equals(quoteMint)) &&
			!(pool.BaseMint.Equals(quoteMint) && pool.QuoteMint.Equals(baseMint)) {
			continue
		}

		poolInfo, err := pm.FetchPoolInfo(ctx, account.Pubkey)
		if err != nil {
			pm.logger.Error("Не удалось получить данные о пуле", zap.String("account", account.Pubkey.String()), zap.Error(err))
			continue
		}
		// Пропускаем пулы с нулевыми резервами
		if poolInfo.BaseReserves == 0 || poolInfo.QuoteReserves == 0 {
			pm.logger.Warn("Пул имеет нулевые резервы, пропуск",
				zap.String("account", account.Pubkey.String()),
				zap.Uint64("base_reserves", poolInfo.BaseReserves),
				zap.Uint64("quote_reserves", poolInfo.QuoteReserves))
			continue
		}

		pm.logger.Info("Найден корректный пул PumpSwap")
		return poolInfo, nil
	}

	pm.logger.Warn("Подходящий пул не найден", zap.String("filter_base_mint", baseMint.String()), zap.String("expected_quote_mint", quoteMint.String()))
	return nil, fmt.Errorf("no matching pool found for %s/%s", baseMint.String(), quoteMint.String())
}

// FetchPoolInfo получает полную информацию о пуле по его адресу.
func (pm *PoolManager) FetchPoolInfo(ctx context.Context, poolAddress solana.PublicKey) (*PoolInfo, error) {
	// Ограничиваем время запроса для получения данных пула
	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Получаем данные пула
	pool, err := pm.getPool(timeoutCtx, poolAddress)
	if err != nil {
		pm.logger.Error("Не удалось получить данные пула", zap.String("pool_address", poolAddress.String()), zap.Error(err))
		return nil, err
	}

	// Получаем глобальную конфигурацию
	config, err := pm.fetchGlobalConfig(timeoutCtx)
	if err != nil {
		pm.logger.Error("Не удалось получить глобальную конфигурацию", zap.Error(err))
		return nil, err
	}

	// Получаем данные токен-аккаунтов
	accounts := []solana.PublicKey{pool.PoolBaseTokenAccount, pool.PoolQuoteTokenAccount}
	accountsData, err := pm.getAccountBinaryDataMultiple(timeoutCtx, accounts)
	if err != nil {
		pm.logger.Error("Не удалось получить данные токен-аккаунтов", zap.Error(err))
		return nil, err
	}

	var baseData, quoteData []byte
	if len(accountsData) >= 1 {
		baseData = accountsData[0]
	}
	if len(accountsData) >= 2 {
		quoteData = accountsData[1]
	}
	baseReserves, quoteReserves := parseTokenAccounts(baseData, quoteData)

	pm.logger.Info("Успешно получена информация о пуле", zap.String("pool_address", poolAddress.String()),
		zap.String("base_mint", pool.BaseMint.String()), zap.String("quote_mint", pool.QuoteMint.String()),
		zap.Uint64("base_reserves", baseReserves), zap.Uint64("quote_reserves", quoteReserves),
		zap.Uint64("lp_supply", pool.LPSupply), zap.Uint64("fees_basis_points", config.LPFeeBasisPoints))

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

// getPool получает и парсит данные пула по адресу.
func (pm *PoolManager) getPool(ctx context.Context, poolAddress solana.PublicKey) (*Pool, error) {
	data, err := pm.getAccountBinaryData(ctx, poolAddress)
	if err != nil {
		return nil, err
	}
	return ParsePool(data)
}

// fetchGlobalConfig получает глобальную конфигурацию программы PumpSwap.
func (pm *PoolManager) fetchGlobalConfig(ctx context.Context) (*GlobalConfig, error) {
	globalConfig, _, err := solana.FindProgramAddress([][]byte{[]byte("global_config")}, pm.programID)
	if err != nil {
		pm.logger.Error("Не удалось вывести адрес глобальной конфигурации", zap.Error(err))
		return nil, fmt.Errorf("failed to derive global config address: %w", err)
	}

	data, err := pm.getAccountBinaryData(ctx, globalConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to get global config account: %w", err)
	}

	config, err := ParseGlobalConfig(data)
	if err != nil {
		pm.logger.Error("Не удалось разобрать глобальную конфигурацию", zap.String("global_config", globalConfig.String()), zap.Error(err))
		return nil, fmt.Errorf("failed to parse global config: %w", err)
	}

	return config, nil
}

// CalculateSwapQuote вычисляет ожидаемый результат обмена в пуле.
func (pm *PoolManager) CalculateSwapQuote(pool *PoolInfo, inputAmount uint64, isBaseToQuote bool) (uint64, float64) {
	feeFactor := 1.0 - (float64(pool.FeesBasisPoints) / 10000.0)
	var output uint64
	var price float64

	if isBaseToQuote {
		output = calculateOutput(pool.BaseReserves, pool.QuoteReserves, inputAmount, feeFactor)
		if inputAmount > 0 {
			price = float64(output) / float64(inputAmount)
		}
	} else {
		output = calculateOutput(pool.QuoteReserves, pool.BaseReserves, inputAmount, feeFactor)
		if output > 0 {
			price = float64(inputAmount) / float64(output)
		}
	}

	return output, price
}

// FindPoolWithRetry ищет пул для пары токенов с повторными попытками.
func (pm *PoolManager) FindPoolWithRetry(ctx context.Context, baseMint, quoteMint solana.PublicKey, maxRetries int, retryDelay time.Duration) (*PoolInfo, error) {

	// Используем значения по умолчанию, если параметры не заданы
	if maxRetries <= 0 {
		maxRetries = pm.maxRetries
	}
	if retryDelay <= 0 {
		retryDelay = pm.retryDelay
	}

	backoffPolicy := backoff.NewExponentialBackOff()
	backoffPolicy.InitialInterval = retryDelay
	backoffPolicy.MaxInterval = retryDelay * 10

	notify := func(err error, duration time.Duration) {
		pm.logger.Info("Повтор попытки после ошибки", zap.Error(err), zap.Duration("backoff", duration))
	}

	operation := func() (*PoolInfo, error) {
		pool, err := pm.FindPool(ctx, baseMint, quoteMint)
		return pool, err
	}

	maxTries := uint(maxRetries)
	pool, err := backoff.Retry(ctx, operation,
		backoff.WithBackOff(backoffPolicy),
		backoff.WithMaxTries(maxTries),
		backoff.WithNotify(notify))

	if err != nil {
		pm.logger.Error("Не удалось найти пул после всех попыток", zap.String("base_mint", baseMint.String()),
			zap.String("quote_mint", quoteMint.String()), zap.Error(err))
		return nil, err
	}

	return pool, nil
}

////////////////////////////////////////////////////////////////////////////////
// Функции, взаимодействующие с DEX
////////////////////////////////////////////////////////////////////////////////

// findAndValidatePool находит и проверяет пул для текущей конфигурации DEX.
func (dex *DEX) findAndValidatePool(ctx context.Context) (*PoolInfo, bool, error) {
	effBase, effQuote := dex.effectiveMints()

	pool, err := dex.poolManager.FindPoolWithRetry(ctx, effBase, effQuote, 5, 2*time.Second)
	if err != nil {
		dex.logger.Error("Не удалось найти пул", zap.String("base_mint", effBase.String()), zap.String("quote_mint", effQuote.String()), zap.Error(err))
		return nil, false, fmt.Errorf("failed to find pool: %w", err)
	}

	dex.config.PoolAddress = pool.Address
	dex.config.LPMint = pool.LPMint

	dex.logger.Info("Получены данные пула", zap.String("pool_address", pool.Address.String()),
		zap.String("base_mint", pool.BaseMint.String()), zap.String("quote_mint", pool.QuoteMint.String()),
		zap.Uint64("base_reserves", pool.BaseReserves), zap.Uint64("quote_reserves", pool.QuoteReserves))

	// Определяем, требуется ли разворот порядка токенов
	poolMintReversed := !pool.BaseMint.Equals(effBase)
	return pool, poolMintReversed, nil
}

////////////////////////////////////////////////////////////////////////////////
// Парсинг бинарных данных пула
////////////////////////////////////////////////////////////////////////////////

// ParsePool парсит бинарные данные аккаунта пула.
func ParsePool(data []byte) (*Pool, error) {
	if len(data) < 8 {
		return nil, fmt.Errorf("data too short for Pool")
	}
	// Проверяем discriminator
	for i := 0; i < 8; i++ {
		if data[i] != PoolDiscriminator[i] {
			return nil, fmt.Errorf("invalid discriminator for Pool")
		}
	}

	pos := 8
	if len(data) < pos+1+2+32*6+8 {
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
