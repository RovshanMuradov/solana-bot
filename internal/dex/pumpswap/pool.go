// =============================
// File: internal/dex/pumpswap/pool.go
// =============================
package pumpswap

import (
	"context"
	"encoding/binary"
	"fmt"
	"golang.org/x/sync/errgroup"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"go.uber.org/zap"

	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc"
)

const (
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

	// кеш глобальной конфигурации
	cfgOnce sync.Once
	cfg     *GlobalConfig
	cfgErr  error
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

// globalConfig возвращает (и кеширует) GlobalConfig.
func (pm *PoolManager) globalConfig(ctx context.Context) (*GlobalConfig, error) {
	pm.cfgOnce.Do(func() {
		pm.cfg, pm.cfgErr = pm.fetchGlobalConfig(ctx)
	})
	return pm.cfg, pm.cfgErr
}

////////////////////////////////////////////////////////////////////////////////
// Вспомогательные функции
////////////////////////////////////////////////////////////////////////////////

// getAccountBinaryData retrieves binary data for a single account with a timeout.
func (pm *PoolManager) getAccountBinaryData(ctx context.Context, pubkey solana.PublicKey) ([]byte, error) {
	// Apply a 5-second timeout to the RPC call
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Fetch account info
	accountInfo, err := pm.client.GetAccountInfo(cctx, pubkey)
	if err != nil {
		pm.logger.Error("GetAccountInfo failed", zap.String("account", pubkey.String()), zap.Error(err))
		return nil, fmt.Errorf("failed to get account info for %s: %w", pubkey.String(), err)
	}
	if accountInfo == nil || accountInfo.Value == nil {
		return nil, fmt.Errorf("account not found: %s", pubkey.String())
	}

	// Return raw binary data
	return accountInfo.Value.Data.GetBinary(), nil
}

// getAccountBinaryDataMultiple retrieves binary data for multiple accounts with a timeout.
func (pm *PoolManager) getAccountBinaryDataMultiple(ctx context.Context, accounts []solana.PublicKey) ([][]byte, error) {
	// Apply a 5-second timeout to the RPC call
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Perform the batch request
	resp, err := pm.client.GetMultipleAccounts(cctx, accounts)
	if err != nil {
		pm.logger.Error("GetMultipleAccounts failed", zap.Error(err))
		return nil, fmt.Errorf("failed to get multiple accounts info: %w", err)
	}

	// Extract binary slices, skipping nil entries
	data := make([][]byte, len(accounts))
	for i, info := range resp.Value {
		if info != nil {
			data[i] = info.Data.GetBinary()
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

// FindPool ищет пул в прямом и обратном порядке параллельно.
func (pm *PoolManager) FindPool(ctx context.Context, baseMint, quoteMint solana.PublicKey) (*PoolInfo, error) {
	searchCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		found *PoolInfo
		mu    sync.Mutex
	)

	g, _ := errgroup.WithContext(searchCtx)

	// прямой порядок
	g.Go(func() error {
		if p, _ := pm.findPoolByProgramAccounts(searchCtx, baseMint, quoteMint); p != nil {
			mu.Lock()
			if found == nil {
				found = p
				cancel()
			}
			mu.Unlock()
		}
		return nil
	})

	// обратный порядок
	g.Go(func() error {
		if p, _ := pm.findPoolByProgramAccounts(searchCtx, quoteMint, baseMint); p != nil {
			// приводим к исходному порядку
			p.BaseMint, p.QuoteMint = p.QuoteMint, p.BaseMint
			p.BaseReserves, p.QuoteReserves = p.QuoteReserves, p.BaseReserves
			p.PoolBaseTokenAccount, p.PoolQuoteTokenAccount = p.PoolQuoteTokenAccount, p.PoolBaseTokenAccount

			mu.Lock()
			if found == nil {
				found = p
				cancel()
			}
			mu.Unlock()
		}
		return nil
	})

	_ = g.Wait()

	if found == nil {
		return nil, fmt.Errorf("no pool found for %s / %s", baseMint, quoteMint)
	}
	return found, nil
}

// findPoolByProgramAccounts ищет пул по паре mint’ов с минимальным числом RPC.
func (pm *PoolManager) findPoolByProgramAccounts(ctx context.Context, baseMint, quoteMint solana.PublicKey) (*PoolInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	const (
		offsetBaseMint  = 8 + 1 + 2 + 32 // 43
		offsetQuoteMint = offsetBaseMint + 32
	)

	opts := &rpc.GetProgramAccountsOpts{
		Commitment: rpc.CommitmentConfirmed,
		Encoding:   solana.EncodingBase64,
		Filters: []rpc.RPCFilter{
			{Memcmp: &rpc.RPCFilterMemcmp{Offset: 0, Bytes: PoolDiscriminator}},
			{Memcmp: &rpc.RPCFilterMemcmp{Offset: offsetBaseMint, Bytes: baseMint.Bytes()}},
			{Memcmp: &rpc.RPCFilterMemcmp{Offset: offsetQuoteMint, Bytes: quoteMint.Bytes()}},
		},
	}

	accounts, err := pm.client.GetProgramAccountsWithOpts(ctx, pm.programID, opts)
	if err != nil {
		return nil, fmt.Errorf("GetProgramAccountsWithOpts: %w", err)
	}
	if len(accounts) == 0 {
		return nil, fmt.Errorf("no program accounts match %s/%s", baseMint, quoteMint)
	}

	// --- получаем весь бинарный контент пулов одним запросом ---
	pubkeys := make([]solana.PublicKey, len(accounts))
	for i, acc := range accounts {
		pubkeys[i] = acc.Pubkey
	}

	poolsRaw, err := pm.getAccountBinaryDataMultiple(ctx, pubkeys)
	if err != nil {
		return nil, err
	}

	// кеш глобальной конфигурации
	cfg, _ := pm.globalConfig(ctx)

	// перебираем кандидатов
	for i, raw := range poolsRaw {
		pool, err := ParsePool(raw)
		if err != nil {
			continue
		}

		// резервы токен‑аккаунтов (два за один запрос)
		tokens := []solana.PublicKey{pool.PoolBaseTokenAccount, pool.PoolQuoteTokenAccount}
		tokRaw, err := pm.getAccountBinaryDataMultiple(ctx, tokens)
		if err != nil {
			continue
		}
		baseRes, quoteRes := parseTokenAccounts(tokRaw[0], tokRaw[1])
		if baseRes == 0 || quoteRes == 0 {
			continue
		}

		return &PoolInfo{
			Address:               pubkeys[i],
			BaseMint:              pool.BaseMint,
			QuoteMint:             pool.QuoteMint,
			BaseReserves:          baseRes,
			QuoteReserves:         quoteRes,
			LPSupply:              pool.LPSupply,
			FeesBasisPoints:       cfg.LPFeeBasisPoints,
			ProtocolFeeBPS:        cfg.ProtocolFeeBasisPoints,
			LPMint:                pool.LPMint,
			PoolBaseTokenAccount:  pool.PoolBaseTokenAccount,
			PoolQuoteTokenAccount: pool.PoolQuoteTokenAccount,
			CoinCreator:           pool.CoinCreator,
		}, nil
	}

	return nil, fmt.Errorf("all candidate pools have zero liquidity for %s/%s", baseMint, quoteMint)
}

// FetchPoolInfo получает полную информацию о пуле по его адресу.
func (pm *PoolManager) FetchPoolInfo(ctx context.Context, poolAddress solana.PublicKey) (*PoolInfo, error) {
	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Данные пула
	pool, err := pm.getPool(timeoutCtx, poolAddress)
	if err != nil {
		pm.logger.Error("Не удалось получить данные пула", zap.String("pool_address", poolAddress.String()), zap.Error(err))
		return nil, err
	}

	// Глобальная конфигурация из кеша
	config, err := pm.globalConfig(timeoutCtx)
	if err != nil {
		pm.logger.Error("Не удалось получить глобальную конфигурацию", zap.Error(err))
		return nil, err
	}

	// Резервы токен‑аккаунтов
	accs := []solana.PublicKey{pool.PoolBaseTokenAccount, pool.PoolQuoteTokenAccount}
	accData, err := pm.getAccountBinaryDataMultiple(timeoutCtx, accs)
	if err != nil {
		pm.logger.Error("Не удалось получить данные токен‑аккаунтов", zap.Error(err))
		return nil, err
	}

	baseRes, quoteRes := parseTokenAccounts(accData[0], accData[1])

	return &PoolInfo{
		Address:               poolAddress,
		BaseMint:              pool.BaseMint,
		QuoteMint:             pool.QuoteMint,
		BaseReserves:          baseRes,
		QuoteReserves:         quoteRes,
		LPSupply:              pool.LPSupply,
		FeesBasisPoints:       config.LPFeeBasisPoints,
		ProtocolFeeBPS:        config.ProtocolFeeBasisPoints,
		LPMint:                pool.LPMint,
		PoolBaseTokenAccount:  pool.PoolBaseTokenAccount,
		PoolQuoteTokenAccount: pool.PoolQuoteTokenAccount,
		CoinCreator:           pool.CoinCreator,
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
func (d *DEX) findAndValidatePool(ctx context.Context) (*PoolInfo, bool, error) {
	effBase, effQuote := d.effectiveMints()

	pool, err := d.poolManager.FindPoolWithRetry(ctx, effBase, effQuote, 5, 2*time.Second)
	if err != nil {
		d.logger.Error("Не удалось найти пул", zap.String("base_mint", effBase.String()), zap.String("quote_mint", effQuote.String()), zap.Error(err))
		return nil, false, fmt.Errorf("failed to find pool: %w", err)
	}

	d.config.PoolAddress = pool.Address
	d.config.LPMint = pool.LPMint

	d.logger.Info("Получены данные пула", zap.String("pool_address", pool.Address.String()),
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
	pos += 8

	// Парсим CoinCreator, если есть
	if len(data) >= pos+32 {
		pool.CoinCreator = solana.PublicKeyFromBytes(data[pos : pos+32])
	}

	return pool, nil
}
