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
	"time"

	"github.com/gagliardetto/solana-go"
	"go.uber.org/zap"

	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc"
)

// Minimum LP supply constant
const MinimumLiquidity uint64 = 1000

// PoolManager handles operations with PumpSwap pools
type PoolManager struct {
	client *solbc.Client
	logger *zap.Logger
}

// NewPoolManager creates a new pool manager
func NewPoolManager(client *solbc.Client, logger *zap.Logger) *PoolManager {
	return &PoolManager{
		client: client,
		logger: logger,
	}
}

// FindPool finds a pool for the given token pair using deterministic discovery
func (pm *PoolManager) FindPool(
	ctx context.Context,
	baseMint, quoteMint solana.PublicKey,
) (*PoolInfo, error) {
	// Create temporary config for pool address derivation
	cfg := &Config{
		ProgramID: PumpSwapProgramID,
		BaseMint:  baseMint,
		QuoteMint: quoteMint,
	}

	// Try different indices starting from 0
	for index := uint16(0); index < 100; index++ {
		// For each index, try with different potential creators
		// First try with the program itself as creator (common pattern)
		possibleCreators := []solana.PublicKey{
			PumpSwapProgramID, // Try program ID as first possible creator
		}

		// Try each possible creator
		for _, creator := range possibleCreators {
			// Derive pool address for this index and creator
			poolAddr, _, err := cfg.DerivePoolAddress(index, creator)
			if err != nil {
				continue
			}

			// Check if the pool account exists
			accountInfo, err := pm.client.GetAccountInfo(ctx, poolAddr)
			if err != nil || accountInfo == nil || accountInfo.Value == nil {
				// Skip if account doesn't exist or error
				continue
			}

			// Check if the account is owned by the PumpSwap program
			if !accountInfo.Value.Owner.Equals(PumpSwapProgramID) {
				continue
			}

			// Try to parse the account data as a Pool
			poolData := accountInfo.Value.Data.GetBinary()
			pool, err := ParsePool(poolData)
			if err != nil {
				pm.logger.Debug("Failed to parse pool data",
					zap.String("pool_address", poolAddr.String()),
					zap.Error(err))
				continue
			}

			// Verify that this pool is for the expected mint pair
			if !pool.BaseMint.Equals(baseMint) || !pool.QuoteMint.Equals(quoteMint) {
				continue
			}

			// Found a valid pool, fetch more pool info
			poolInfo, err := pm.FetchPoolInfo(ctx, poolAddr)
			if err != nil {
				pm.logger.Error("Failed to fetch pool info",
					zap.String("pool_address", poolAddr.String()),
					zap.Error(err))
				continue
			}

			pm.logger.Info("Found PumpSwap pool",
				zap.String("pool_address", poolAddr.String()),
				zap.String("base_mint", pool.BaseMint.String()),
				zap.String("quote_mint", pool.QuoteMint.String()))

			return poolInfo, nil
		}
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
		// Try to find the pool
		poolInfo, err := pm.FindPool(ctx, baseMint, quoteMint)
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
