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
	"math/big"
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

// PoolCache stores found pools for quick access
type PoolCache struct {
	mutex      sync.RWMutex
	pools      map[string]*PoolInfo // key: hashed pool key
	expiration map[string]time.Time
	ttl        time.Duration
}

// NewPoolCache creates a new pool cache with specified TTL
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

// Get retrieves a pool from cache, checking expiration
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

// Set adds a pool to the cache with expiration time
func (pc *PoolCache) Set(baseMint, quoteMint solana.PublicKey, pool *PoolInfo) {
	pc.mutex.Lock()
	defer pc.mutex.Unlock()

	pc.cleanExpired()

	key := makePoolCacheKey(baseMint, quoteMint)
	pc.pools[key] = pool
	pc.expiration[key] = time.Now().Add(pc.ttl)
}

// cleanExpired removes expired entries from cache
func (pc *PoolCache) cleanExpired() {
	now := time.Now()
	for key, expiry := range pc.expiration {
		if now.After(expiry) {
			delete(pc.pools, key)
			delete(pc.expiration, key)
		}
	}
}

func makePoolCacheKey(baseMint, quoteMint solana.PublicKey) string {
	// Sort mints for consistent key regardless of order
	if baseMint.String() < quoteMint.String() {
		return fmt.Sprintf("%s:%s", baseMint, quoteMint)
	}
	return fmt.Sprintf("%s:%s", quoteMint, baseMint)
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

// FindPool finds a pool for the given token pair
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

func calculateOutput(reserves, otherReserves, amount uint64, feeFactor float64) uint64 {
	x := new(big.Float).SetUint64(reserves)
	y := new(big.Float).SetUint64(otherReserves)
	a := new(big.Float).SetUint64(amount)

	// Apply fee to input amount
	a.Mul(a, big.NewFloat(feeFactor))

	// Formula: outputAmount = y * a / (x + a)
	numerator := new(big.Float).Mul(y, a)
	denominator := new(big.Float).Add(x, a)
	result := new(big.Float).Quo(numerator, denominator)

	output, _ := result.Uint64()
	return output
}

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
