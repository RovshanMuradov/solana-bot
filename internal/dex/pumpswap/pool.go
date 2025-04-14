// =============================
// File: internal/dex/pumpswap/pool.go
// =============================
package pumpswap

import (
	"context"
	"encoding/binary"
	"fmt"
	"github.com/cenkalti/backoff/v5"
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
	FetchPoolInfo(ctx context.Context, poolAddress solana.PublicKey) (*PoolInfo, error)
}

// PoolManager handles operations with PumpSwap pools
type PoolManager struct {
	client     *solbc.Client
	logger     *zap.Logger
	programID  solana.PublicKey
	maxRetries int
	retryDelay time.Duration
}

// PoolManagerOptions contains options for creating a PoolManager
type PoolManagerOptions struct {
	MaxRetries int
	RetryDelay time.Duration
	ProgramID  solana.PublicKey
}

// DefaultPoolManagerOptions возвращает настройки по умолчанию для менеджера пулов.
func DefaultPoolManagerOptions() PoolManagerOptions {
	return PoolManagerOptions{
		MaxRetries: 3,
		RetryDelay: time.Second,
		ProgramID:  PumpSwapProgramID,
	}
}

// NewPoolManager создает новый менеджер пулов с указанными опциями.
func NewPoolManager(client *solbc.Client, logger *zap.Logger, opts ...PoolManagerOptions) *PoolManager {
	defaultOpts := DefaultPoolManagerOptions()

	var options PoolManagerOptions
	if len(opts) > 0 {
		options = opts[0]
	} else {
		options = defaultOpts
	}

	logger.Info("Creating new pool manager",
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

// FindPool находит пул для заданной пары токенов.
func (pm *PoolManager) FindPool(
	ctx context.Context,
	baseMint, quoteMint solana.PublicKey,
) (*PoolInfo, error) {
	pm.logger.Info("Starting FindPool method",
		zap.String("base_mint", baseMint.String()),
		zap.String("quote_mint", quoteMint.String()))

	pool, err := pm.findPoolByProgramAccounts(ctx, baseMint, quoteMint)
	if err == nil && pool != nil {
		pm.logger.Info("Found pool in direct order",
			zap.String("pool_address", pool.Address.String()))
		return pool, nil
	}

	pm.logger.Info("No pool found in direct order, trying reverse order",
		zap.Error(err))

	pool, err = pm.findPoolByProgramAccounts(ctx, quoteMint, baseMint)
	if err == nil && pool != nil {
		pm.logger.Info("Found pool in reverse order",
			zap.String("pool_address", pool.Address.String()))

		pool.BaseMint, pool.QuoteMint = pool.QuoteMint, pool.BaseMint
		pool.BaseReserves, pool.QuoteReserves = pool.QuoteReserves, pool.BaseReserves
		pool.PoolBaseTokenAccount, pool.PoolQuoteTokenAccount = pool.PoolQuoteTokenAccount, pool.PoolBaseTokenAccount

		return pool, nil
	}

	pm.logger.Error("No pool found in either order",
		zap.String("base_mint", baseMint.String()),
		zap.String("quote_mint", quoteMint.String()),
		zap.Error(err))

	return nil, fmt.Errorf("no pool found for base mint %s and quote mint %s",
		baseMint.String(), quoteMint.String())
}

// findPoolByProgramAccounts ищет пул через сканирование программных аккаунтов.
func (pm *PoolManager) findPoolByProgramAccounts(
	ctx context.Context,
	baseMint, quoteMint solana.PublicKey,
) (*PoolInfo, error) {
	pm.logger.Info("Starting findPoolByProgramAccounts",
		zap.String("base_mint", baseMint.String()),
		zap.String("quote_mint", quoteMint.String()))

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

	pm.logger.Debug("Calling GetProgramAccountsWithOpts",
		zap.String("program_id", pm.programID.String()))

	accounts, err := pm.client.GetProgramAccountsWithOpts(ctx, pm.programID, opts)
	if err != nil {
		pm.logger.Error("Failed to get program accounts",
			zap.Error(err))
		return nil, fmt.Errorf("failed to get program accounts: %w", err)
	}

	pm.logger.Info("Retrieved program accounts",
		zap.Int("account_count", len(accounts)))

	for i, account := range accounts {
		pm.logger.Debug("Processing account",
			zap.Int("index", i),
			zap.String("pubkey", account.Pubkey.String()))

		poolData := account.Account.Data.GetBinary()
		pool, err := ParsePool(poolData)
		if err != nil {
			pm.logger.Debug("Failed to parse pool data",
				zap.String("account", account.Pubkey.String()),
				zap.Error(err))
			continue
		}

		// Check if pool matches our token pair
		isMatch := (pool.BaseMint.Equals(baseMint) && pool.QuoteMint.Equals(quoteMint)) ||
			(pool.BaseMint.Equals(quoteMint) && pool.QuoteMint.Equals(baseMint))

		if !isMatch {
			pm.logger.Debug("Pool doesn't match token pair",
				zap.String("account", account.Pubkey.String()),
				zap.String("pool_base_mint", pool.BaseMint.String()),
				zap.String("pool_quote_mint", pool.QuoteMint.String()))
			continue
		}

		pm.logger.Info("Found matching pool, fetching pool info",
			zap.String("account", account.Pubkey.String()))

		poolInfo, err := pm.FetchPoolInfo(ctx, account.Pubkey)
		if err != nil {
			pm.logger.Error("Failed to fetch pool info",
				zap.String("account", account.Pubkey.String()),
				zap.Error(err))
			continue
		}

		if poolInfo.BaseReserves == 0 || poolInfo.QuoteReserves == 0 {
			pm.logger.Warn("Pool has zero reserves, skipping",
				zap.String("account", account.Pubkey.String()),
				zap.Uint64("base_reserves", poolInfo.BaseReserves),
				zap.Uint64("quote_reserves", poolInfo.QuoteReserves))
			continue
		}

		pm.logger.Info("Found valid PumpSwap pool",
			zap.String("pool_address", account.Pubkey.String()),
			zap.String("base_mint", pool.BaseMint.String()),
			zap.String("quote_mint", pool.QuoteMint.String()),
			zap.Uint64("base_reserves", poolInfo.BaseReserves),
			zap.Uint64("quote_reserves", poolInfo.QuoteReserves))

		return poolInfo, nil
	}

	pm.logger.Warn("No matching pool found",
		zap.String("base_mint", baseMint.String()),
		zap.String("quote_mint", quoteMint.String()))

	return nil, fmt.Errorf("no matching pool found for %s/%s",
		baseMint.String(), quoteMint.String())
}

// fetchGlobalConfig получает глобальную конфигурацию программы PumpSwap.
func (pm *PoolManager) fetchGlobalConfig(ctx context.Context) (*GlobalConfig, error) {
	pm.logger.Debug("Deriving global config address")

	globalConfig, _, err := solana.FindProgramAddress(
		[][]byte{[]byte("global_config")},
		pm.programID,
	)
	if err != nil {
		pm.logger.Error("Failed to derive global config address",
			zap.Error(err))
		return nil, fmt.Errorf("failed to derive global config address: %w", err)
	}

	pm.logger.Debug("Getting global config account info",
		zap.String("global_config", globalConfig.String()))

	accountInfo, err := pm.client.GetAccountInfo(ctx, globalConfig)
	if err != nil {
		pm.logger.Error("Failed to get global config account",
			zap.String("global_config", globalConfig.String()),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get global config account: %w", err)
	}
	if accountInfo == nil || accountInfo.Value == nil {
		pm.logger.Error("Global config account not found",
			zap.String("global_config", globalConfig.String()))
		return nil, fmt.Errorf("global config account not found: %s", globalConfig.String())
	}

	pm.logger.Debug("Parsing global config data",
		zap.String("global_config", globalConfig.String()),
		zap.Int("data_size", len(accountInfo.Value.Data.GetBinary())))

	globalData := accountInfo.Value.Data.GetBinary()
	config, err := ParseGlobalConfig(globalData)
	if err != nil {
		pm.logger.Error("Failed to parse global config",
			zap.String("global_config", globalConfig.String()),
			zap.Error(err))
		return nil, fmt.Errorf("failed to parse global config: %w", err)
	}

	pm.logger.Debug("Global config parsed successfully",
		zap.String("global_config", globalConfig.String()),
		zap.String("admin", config.Admin.String()),
		zap.Uint64("lp_fee_bps", config.LPFeeBasisPoints),
		zap.Uint64("protocol_fee_bps", config.ProtocolFeeBasisPoints))

	return config, nil
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

// getPool извлекает и парсит данные пула по адресу.
func (pm *PoolManager) getPool(ctx context.Context, poolAddress solana.PublicKey) (*Pool, error) {
	pm.logger.Debug("Getting pool account info",
		zap.String("pool_address", poolAddress.String()))

	accountInfo, err := pm.client.GetAccountInfo(ctx, poolAddress)
	if err != nil {
		pm.logger.Error("Failed to get pool account info",
			zap.String("pool_address", poolAddress.String()),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get pool account: %w", err)
	}
	if accountInfo == nil || accountInfo.Value == nil {
		pm.logger.Error("Pool account not found",
			zap.String("pool_address", poolAddress.String()))
		return nil, fmt.Errorf("pool account not found: %s", poolAddress.String())
	}

	pm.logger.Debug("Parsing pool data",
		zap.String("pool_address", poolAddress.String()),
		zap.Int("data_size", len(accountInfo.Value.Data.GetBinary())))

	return ParsePool(accountInfo.Value.Data.GetBinary())
}

// getTokenAccountsData получает бинарные данные для заданных аккаунтов.
func (pm *PoolManager) getTokenAccountsData(
	ctx context.Context, accounts []solana.PublicKey,
) ([][]byte, error) {
	accountsStr := make([]string, len(accounts))
	for i, acc := range accounts {
		accountsStr[i] = acc.String()
	}

	pm.logger.Debug("Getting multiple accounts data",
		zap.Strings("accounts", accountsStr))

	accountsInfo, err := pm.client.GetMultipleAccounts(ctx, accounts)
	if err != nil {
		pm.logger.Error("Failed to get accounts info",
			zap.Strings("accounts", accountsStr),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get accounts info: %w", err)
	}
	if accountsInfo == nil || accountsInfo.Value == nil || len(accountsInfo.Value) < len(accounts) {
		pm.logger.Error("Failed to get required token accounts",
			zap.Strings("accounts", accountsStr),
			zap.Int("requested", len(accounts)),
			zap.Int("received", len(accountsInfo.Value)))
		return nil, fmt.Errorf("failed to get required token accounts")
	}

	data := make([][]byte, len(accounts))
	for i, info := range accountsInfo.Value {
		if info != nil {
			data[i] = info.Data.GetBinary()
			pm.logger.Debug("Token account data received",
				zap.String("account", accounts[i].String()),
				zap.Int("data_size", len(data[i])))
		} else {
			pm.logger.Warn("Token account data is nil",
				zap.String("account", accounts[i].String()))
		}
	}
	return data, nil
}

// FetchPoolInfo получает полную информацию о пуле по его адресу.
func (pm *PoolManager) FetchPoolInfo(
	ctx context.Context,
	poolAddress solana.PublicKey,
) (*PoolInfo, error) {
	pm.logger.Debug("Starting FetchPoolInfo",
		zap.String("pool_address", poolAddress.String()))

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	pm.logger.Debug("Getting pool data")
	pool, err := pm.getPool(ctx, poolAddress)
	if err != nil {
		pm.logger.Error("Failed to get pool data",
			zap.String("pool_address", poolAddress.String()),
			zap.Error(err))
		return nil, err
	}

	pm.logger.Debug("Getting global config")
	config, err := pm.fetchGlobalConfig(ctx)
	if err != nil {
		pm.logger.Error("Failed to fetch global config",
			zap.Error(err))
		return nil, err
	}

	accounts := []solana.PublicKey{
		pool.PoolBaseTokenAccount,
		pool.PoolQuoteTokenAccount,
	}

	pm.logger.Debug("Getting token accounts data",
		zap.String("base_token_account", pool.PoolBaseTokenAccount.String()),
		zap.String("quote_token_account", pool.PoolQuoteTokenAccount.String()))

	accountsData, err := pm.getTokenAccountsData(ctx, accounts)
	if err != nil {
		pm.logger.Error("Failed to get token accounts data",
			zap.Error(err))
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

	pm.logger.Info("Pool info fetched successfully",
		zap.String("pool_address", poolAddress.String()),
		zap.String("base_mint", pool.BaseMint.String()),
		zap.String("quote_mint", pool.QuoteMint.String()),
		zap.Uint64("base_reserves", baseReserves),
		zap.Uint64("quote_reserves", quoteReserves),
		zap.Uint64("lp_supply", pool.LPSupply),
		zap.Uint64("fees_basis_points", config.LPFeeBasisPoints))

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
func (pm *PoolManager) CalculateSwapQuote(pool *PoolInfo, inputAmount uint64, isBaseToQuote bool) (uint64, float64) {
	pm.logger.Debug("Calculating swap quote",
		zap.String("pool_address", pool.Address.String()),
		zap.Uint64("input_amount", inputAmount),
		zap.Bool("is_base_to_quote", isBaseToQuote))

	feeFactor := 1.0 - (float64(pool.FeesBasisPoints) / 10000.0)

	var output uint64
	var price float64

	if isBaseToQuote {
		output = calculateOutput(pool.BaseReserves, pool.QuoteReserves, inputAmount, feeFactor)
		price = float64(0)
		if inputAmount > 0 {
			price = float64(output) / float64(inputAmount)
		}
	} else {
		output = calculateOutput(pool.QuoteReserves, pool.BaseReserves, inputAmount, feeFactor)
		price = float64(0)
		if output > 0 {
			price = float64(inputAmount) / float64(output)
		}
	}

	pm.logger.Debug("Swap quote calculated",
		zap.Uint64("input_amount", inputAmount),
		zap.Uint64("output_amount", output),
		zap.Float64("price", price),
		zap.Bool("is_base_to_quote", isBaseToQuote))

	return output, price
}

// FindPoolWithRetry ищет пул для пары токенов с повторными попытками.
func (pm *PoolManager) FindPoolWithRetry(
	ctx context.Context,
	baseMint, quoteMint solana.PublicKey,
	maxRetries int,
	retryDelay time.Duration,
) (*PoolInfo, error) {
	pm.logger.Info("Starting FindPoolWithRetry",
		zap.String("base_mint", baseMint.String()),
		zap.String("quote_mint", quoteMint.String()),
		zap.Int("max_retries", maxRetries),
		zap.Duration("retry_delay", retryDelay))

	if maxRetries <= 0 {
		maxRetries = pm.maxRetries
		pm.logger.Debug("Using default max retries", zap.Int("max_retries", maxRetries))
	}
	if retryDelay <= 0 {
		retryDelay = pm.retryDelay
		pm.logger.Debug("Using default retry delay", zap.Duration("retry_delay", retryDelay))
	}

	backoffPolicy := backoff.NewExponentialBackOff()
	backoffPolicy.InitialInterval = retryDelay
	backoffPolicy.MaxInterval = retryDelay * 10

	pm.logger.Debug("Configured backoff policy",
		zap.Duration("initial_interval", backoffPolicy.InitialInterval),
		zap.Duration("max_interval", backoffPolicy.MaxInterval))

	// Create a properly typed operation function
	operation := func() (*PoolInfo, error) {
		pm.logger.Debug("Executing FindPool in retry operation")
		pool, err := pm.FindPool(ctx, baseMint, quoteMint)
		if err != nil {
			pm.logger.Debug("Failed to find pool, will retry",
				zap.String("base", baseMint.String()),
				zap.String("quote", quoteMint.String()),
				zap.Error(err))
			return nil, err
		}
		pm.logger.Debug("Pool found in retry operation",
			zap.String("pool_address", pool.Address.String()))
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

	pm.logger.Debug("Starting retry operation", zap.Uint("max_tries", maxTriesUint))

	// Use the Retry function with correct options
	pool, err := backoff.Retry(
		ctx,
		operation,
		backoff.WithBackOff(backoffPolicy),
		backoff.WithMaxTries(maxTriesUint),
		backoff.WithNotify(notify),
	)

	if err != nil {
		pm.logger.Error("Failed to find pool after all retries",
			zap.String("base_mint", baseMint.String()),
			zap.String("quote_mint", quoteMint.String()),
			zap.Error(err))
		return nil, err
	}

	pm.logger.Info("Successfully found pool with retries",
		zap.String("pool_address", pool.Address.String()),
		zap.String("base_mint", pool.BaseMint.String()),
		zap.String("quote_mint", pool.QuoteMint.String()))

	return pool, nil
}

// findAndValidatePool находит и проверяет пул для текущей конфигурации DEX.
func (dex *DEX) findAndValidatePool(ctx context.Context) (*PoolInfo, bool, error) {
	dex.logger.Info("Starting findAndValidatePool")
	effBase, effQuote := dex.effectiveMints()
	dex.logger.Info("Determined effective mints",
		zap.String("effective_base", effBase.String()),
		zap.String("effective_quote", effQuote.String()))

	pool, err := dex.poolManager.FindPoolWithRetry(ctx, effBase, effQuote, 5, 2*time.Second)
	if err != nil {
		dex.logger.Error("Failed to find pool",
			zap.String("base_mint", effBase.String()),
			zap.String("quote_mint", effQuote.String()),
			zap.Error(err))
		return nil, false, fmt.Errorf("failed to find pool: %w", err)
	}

	dex.config.PoolAddress = pool.Address
	dex.config.LPMint = pool.LPMint

	dex.logger.Info("Found pool details",
		zap.String("pool_address", pool.Address.String()),
		zap.String("base_mint", pool.BaseMint.String()),
		zap.String("quote_mint", pool.QuoteMint.String()),
		zap.Uint64("base_reserves", pool.BaseReserves),
		zap.Uint64("quote_reserves", pool.QuoteReserves))

	poolMintReversed := !pool.BaseMint.Equals(effBase)
	dex.logger.Info("Pool mint order",
		zap.Bool("is_reversed", poolMintReversed))

	return pool, poolMintReversed, nil
}

// ParsePool парсит бинарные данные аккаунта пула.
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
