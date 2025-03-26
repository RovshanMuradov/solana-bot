// =============================
// File: internal/dex/pumpswap/pool.go
// =============================
package pumpswap

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"math/big"
	"sync"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"go.uber.org/zap"

	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc"
)

// Minimum LP supply constant
const MinimumLiquidity uint64 = 1000

// PoolCache stores found pools for quick access
type PoolCache struct {
	mutex      sync.RWMutex
	pools      map[string]*PoolInfo // key: baseMint:quoteMint
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
		// Expired but will remove later (during write)
		return nil, false
	}

	return pool, true
}

// Set adds a pool to the cache with expiration time
func (pc *PoolCache) Set(baseMint, quoteMint solana.PublicKey, pool *PoolInfo) {
	pc.mutex.Lock()
	defer pc.mutex.Unlock()

	// Clean expired entries when adding new ones
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

// makePoolCacheKey creates a key for the pool cache
func makePoolCacheKey(baseMint, quoteMint solana.PublicKey) string {
	// Always sort mints for consistent key, regardless of order
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
	// 1. First check cache (for both directions of the pair)
	if pool, found := pm.cache.Get(baseMint, quoteMint); found {
		pm.logger.Debug("Found pool in cache",
			zap.String("base_mint", baseMint.String()),
			zap.String("quote_mint", quoteMint.String()))
		return pool, nil
	}

	// Check in reverse order
	if pool, found := pm.cache.Get(quoteMint, baseMint); found {
		pm.logger.Debug("Found pool in cache (reversed order)",
			zap.String("base_mint", quoteMint.String()),
			zap.String("quote_mint", baseMint.String()))

		// Swap tokens in result for consistency with request
		pool.BaseMint, pool.QuoteMint = pool.QuoteMint, pool.BaseMint
		pool.BaseReserves, pool.QuoteReserves = pool.QuoteReserves, pool.BaseReserves
		pool.PoolBaseTokenAccount, pool.PoolQuoteTokenAccount = pool.PoolQuoteTokenAccount, pool.PoolBaseTokenAccount
		return pool, nil
	}

	// 2. Direct lookup via program accounts
	pool, err := pm.findPoolByProgramAccounts(ctx, baseMint, quoteMint)
	if err == nil && pool != nil {
		pm.cache.Set(baseMint, quoteMint, pool)
		return pool, nil
	}

	// 3. Try finding in reverse order
	pool, err = pm.findPoolByProgramAccounts(ctx, quoteMint, baseMint)
	if err == nil && pool != nil {
		// Swap tokens in result for consistency with request
		pool.BaseMint, pool.QuoteMint = pool.QuoteMint, pool.BaseMint
		pool.BaseReserves, pool.QuoteReserves = pool.QuoteReserves, pool.BaseReserves
		pool.PoolBaseTokenAccount, pool.PoolQuoteTokenAccount = pool.PoolQuoteTokenAccount, pool.PoolBaseTokenAccount

		pm.cache.Set(baseMint, quoteMint, pool)
		return pool, nil
	}

	return nil, fmt.Errorf("no pool found for base mint %s and quote mint %s",
		baseMint.String(), quoteMint.String())
}

// isPoolDiscriminator checks if discriminator matches pool discriminator
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

// findPoolByProgramAccounts looks for a pool using GetProgramAccounts with filters
func (pm *PoolManager) findPoolByProgramAccounts(
	ctx context.Context,
	baseMint, quoteMint solana.PublicKey,
) (*PoolInfo, error) {
	// Set timeout for this operation
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Create query options with discriminator filter
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

	// Use client method
	accounts, err := pm.client.GetProgramAccountsWithOpts(ctx, pm.programID, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to get program accounts: %w", err)
	}

	// Process all pools
	for _, account := range accounts {
		poolData := account.Account.Data.GetBinary()
		pool, err := ParsePool(poolData)
		if err != nil {
			continue
		}

		// Check if pool matches the token pair
		if (pool.BaseMint.Equals(baseMint) && pool.QuoteMint.Equals(quoteMint)) ||
			(pool.BaseMint.Equals(quoteMint) && pool.QuoteMint.Equals(baseMint)) {

			// Found matching pool
			poolInfo, err := pm.FetchPoolInfo(ctx, account.Pubkey)
			if err != nil {
				continue
			}

			// Check for liquidity
			if poolInfo.BaseReserves == 0 || poolInfo.QuoteReserves == 0 {
				continue
			}

			pm.logger.Info("Found PumpSwap pool",
				zap.String("pool_address", account.Pubkey.String()),
				zap.String("base_mint", pool.BaseMint.String()),
				zap.String("quote_mint", pool.QuoteMint.String()))

			return poolInfo, nil
		}
	}

	return nil, fmt.Errorf("no matching pool found for %s/%s",
		baseMint.String(), quoteMint.String())
}

// FetchPoolInfo fetches detailed pool information using batch request
func (pm *PoolManager) FetchPoolInfo(
	ctx context.Context,
	poolAddress solana.PublicKey,
) (*PoolInfo, error) {
	// Set timeout for this operation
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

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

	// Derive global config address
	globalConfig, _, err := solana.FindProgramAddress(
		[][]byte{[]byte("global_config")},
		pm.programID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to derive global config address: %w", err)
	}

	// Batch request for all accounts we need in one call
	accounts := []solana.PublicKey{
		globalConfig,
		pool.PoolBaseTokenAccount,
		pool.PoolQuoteTokenAccount,
	}

	accountsInfo, err := pm.client.GetMultipleAccounts(ctx, accounts)
	if err != nil {
		return nil, fmt.Errorf("failed to get accounts info: %w", err)
	}

	if accountsInfo == nil || accountsInfo.Value == nil || len(accountsInfo.Value) < 3 {
		return nil, fmt.Errorf("failed to get required accounts")
	}

	// Check global config
	if accountsInfo.Value[0] == nil {
		return nil, fmt.Errorf("global config account not found: %s", globalConfig.String())
	}

	// Parse global config
	globalData := accountsInfo.Value[0].Data.GetBinary()
	config, err := ParseGlobalConfig(globalData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse global config: %w", err)
	}

	// Parse token accounts for reserves
	var baseReserves, quoteReserves uint64

	// Parse base token account
	if accountsInfo.Value[1] != nil {
		baseData := accountsInfo.Value[1].Data.GetBinary()
		if len(baseData) >= 72 {
			baseReserves = binary.LittleEndian.Uint64(baseData[64:72])
		}
	}

	// Parse quote token account
	if accountsInfo.Value[2] != nil {
		quoteData := accountsInfo.Value[2].Data.GetBinary()
		if len(quoteData) >= 72 {
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
	if maxRetries <= 0 {
		maxRetries = pm.maxRetries
	}

	if retryDelay <= 0 {
		retryDelay = pm.retryDelay
	}

	var errs []error

	for attempt := 0; attempt < maxRetries; attempt++ {
		// Create timeout context for each attempt
		searchCtx, cancel := context.WithTimeout(ctx, retryDelay*2)

		// Try to find the pool
		poolInfo, err := pm.FindPool(searchCtx, baseMint, quoteMint)
		cancel() // Cancel search context

		if err == nil {
			return poolInfo, nil
		}

		errs = append(errs, err)

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

	return nil, fmt.Errorf("failed to find pool after %d attempts for %s/%s: %w",
		maxRetries, baseMint.String(), quoteMint.String(), errors.Join(errs...))
}

// DerivePoolAddress calculates PDA for pool with given parameters
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

// findAndValidatePool finds a pool for the effective pair (baseMint, quoteMint) and validates
// that the found pool matches expected values (base mint should match)
func (dex *DEX) findAndValidatePool(ctx context.Context) (*PoolInfo, bool, error) {
	// Get effective mints for swap
	effBase, effQuote := dex.effectiveMints()

	// Find pool with retries
	pool, err := dex.poolManager.FindPoolWithRetry(ctx, effBase, effQuote, 5, 2*time.Second)
	if err != nil {
		return nil, false, fmt.Errorf("failed to find pool: %w", err)
	}

	// Update configuration (pool address and LP token)
	dex.config.PoolAddress = pool.Address
	dex.config.LPMint = pool.LPMint

	dex.logger.Debug("Found pool details",
		zap.String("pool_address", pool.Address.String()),
		zap.String("base_mint", pool.BaseMint.String()),
		zap.String("quote_mint", pool.QuoteMint.String()))

	// If pool found in reverse order, return poolMintReversed = true
	poolMintReversed := false
	if !pool.BaseMint.Equals(effBase) {
		poolMintReversed = true
	}

	return pool, poolMintReversed, nil
}

// ParsePool parses account data into Pool structure
func ParsePool(data []byte) (*Pool, error) {
	if len(data) < 8 {
		return nil, fmt.Errorf("data too short for Pool")
	}

	// Check discriminator
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
