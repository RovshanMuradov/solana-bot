// internal/dex/raydium/sniper.go
package raydium

import (
	"context"
	"fmt"
	"github.com/gagliardetto/solana-go"
	"go.uber.org/zap"
)

// Snipe executes a fast buy of a new token on Raydium if a pool is found.
func (c *Client) Snipe(ctx context.Context, snipeParams *SnipeParams) (*SwapResult, error) {
	// 1. Find or confirm the pool
	poolAddr, err := FindRaydiumPoolForNewToken(ctx, snipeParams.TokenMint.String(), c.logger)
	if err != nil {
		return nil, fmt.Errorf("failed to find pool for token: %w", err)
	}

	// 2. Set up basic SwapParams
	swapParams := &SwapParams{
		PoolAddress:                 solana.MustPublicKeyFromBase58(poolAddr),
		AmmAuthority:                snipeParams.AmmAuthority,
		BaseVault:                   snipeParams.BaseVault,
		QuoteVault:                  snipeParams.QuoteVault,
		SourceMint:                  snipeParams.SourceMint,
		TargetMint:                  snipeParams.TokenMint,
		UserPublicKey:               snipeParams.UserPublicKey,
		PrivateKey:                  snipeParams.PrivateKey,
		UserSourceTokenAccount:      snipeParams.UserSourceATA,
		UserDestinationTokenAccount: snipeParams.UserDestATA,
		AmountInLamports:            snipeParams.AmountInLamports,
		MinAmountOut:                snipeParams.MinOutLamports,
		Direction:                   0, // e.g. 0: base->quote or 1: quote->base
		ComputeUnits:                800000,
		PriorityFeeLamports:         snipeParams.PriorityFeeLamports,
		WaitForConfirmation:         true,
	}

	// 3. Execute swap
	result, err := c.Swap(ctx, swapParams)
	if err != nil {
		return nil, fmt.Errorf("snipe swap failed: %w", err)
	}
	c.logger.Info("Snipe completed", zap.String("signature", result.Signature.String()))
	return result, nil
}
