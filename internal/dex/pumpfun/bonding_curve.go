// ==============================================
// File: internal/dex/pumpfun/bonding_curve.go
// ==============================================
package pumpfun

import (
	"context"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc"
	"go.uber.org/zap"
)

// QueryBondingCurveState queries the bonding curve account for its current state
func QueryBondingCurveState(ctx context.Context, client *solbc.Client, bondingCurveAddr solana.PublicKey, _ *zap.Logger) (*BondingCurveInfo, error) {
	accountInfo, err := client.GetAccountInfo(ctx, bondingCurveAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to get bonding curve account info: %w", err)
	}

	if accountInfo == nil || accountInfo.Value == nil {
		return nil, fmt.Errorf("bonding curve account not found at %s", bondingCurveAddr.String())
	}

	data := accountInfo.Value.Data.GetBinary()

	if len(data) < 24 {
		return nil, fmt.Errorf("insufficient bonding curve data length: %d", len(data))
	}

	totalSOLLamports := binary.LittleEndian.Uint64(data[0:8])
	progressRaw := binary.LittleEndian.Uint64(data[8:16])
	marketCapLamports := binary.LittleEndian.Uint64(data[16:24])

	totalSOL := float64(totalSOLLamports) / 1e9
	progress := float64(progressRaw) / 100.0
	marketCap := float64(marketCapLamports) / 1e9

	return &BondingCurveInfo{
		Progress:    progress,
		TotalSOL:    totalSOL,
		MarketCap:   marketCap,
		LastUpdated: time.Now(),
	}, nil
}

// DiscoverAssociatedBondingCurve uses the correct FindAssociatedTokenAddress method 
// to directly find the associated bonding curve account for a given bonding curve.
// This directly aligns with the Pump.fun protocol requirements for address derivation.
func DiscoverAssociatedBondingCurve(
	ctx context.Context,
	client *solbc.Client,
	bondingCurve solana.PublicKey,
	mint solana.PublicKey, // Added mint parameter to fix the error - mint is required
	logger *zap.Logger,
) (solana.PublicKey, error) {
	logger.Debug("Finding associated bonding curve using token program derivation",
		zap.String("bonding_curve", bondingCurve.String()),
		zap.String("mint", mint.String()))

	// Use the official solana.FindAssociatedTokenAddress method which correctly derives
	// the associated token address according to the Solana Associated Token Account Program
	associatedBondingCurve, _, err := solana.FindAssociatedTokenAddress(bondingCurve, mint)
	if err != nil {
		logger.Error("Failed to find associated bonding curve address",
			zap.String("bonding_curve", bondingCurve.String()),
			zap.Error(err))
		return solana.PublicKey{}, fmt.Errorf("failed to find associated bonding curve: %w", err)
	}

	// Verify the account exists and is owned by the expected program
	accountInfo, err := client.GetAccountInfo(ctx, associatedBondingCurve)
	if err != nil {
		logger.Warn("Could not verify associated bonding curve account",
			zap.String("address", associatedBondingCurve.String()),
			zap.Error(err))
		// Don't return error here - the account might not exist yet but the address is still valid
	} else if accountInfo != nil && accountInfo.Value != nil && !accountInfo.Value.Owner.Equals(solana.TokenProgramID) {
		logger.Warn("Associated bonding curve exists but has unexpected owner",
			zap.String("address", associatedBondingCurve.String()),
			zap.String("actual_owner", accountInfo.Value.Owner.String()),
			zap.String("expected_owner", solana.TokenProgramID.String()))
	}

	logger.Info("Found associated bonding curve address",
		zap.String("address", associatedBondingCurve.String()))
	
	return associatedBondingCurve, nil
}

// Note: We've removed the getMintFromBondingCurve helper function
// since we now pass the mint directly to the DiscoverAssociatedBondingCurve function

// IsTokenEligibleForPumpfun verifies if a token is compatible with Pump.fun operations
// using the correct account derivation method
func IsTokenEligibleForPumpfun(
	ctx context.Context,
	client *solbc.Client,
	mint solana.PublicKey,
	bondingCurve solana.PublicKey,
	programID solana.PublicKey,
	logger *zap.Logger,
) (bool, solana.PublicKey, error) {
	// Step 1: Verify bonding curve exists and is properly owned
	bcInfo, err := client.GetAccountInfo(ctx, bondingCurve)
	if err != nil || bcInfo == nil || bcInfo.Value == nil {
		logger.Warn("Bonding curve does not exist or cannot be accessed",
			zap.String("bonding_curve", bondingCurve.String()),
			zap.Error(err))
		return false, solana.PublicKey{}, fmt.Errorf("bonding curve not found")
	}

	// Check ownership
	if !bcInfo.Value.Owner.Equals(programID) {
		logger.Warn("Bonding curve has incorrect ownership",
			zap.String("bonding_curve", bondingCurve.String()),
			zap.String("owner", bcInfo.Value.Owner.String()),
			zap.String("expected", programID.String()))
		return false, solana.PublicKey{}, fmt.Errorf("bonding curve has incorrect ownership")
	}

	// Step 2: Directly derive the associated token address - this is the key fix
	associatedBondingCurve, _, err := solana.FindAssociatedTokenAddress(bondingCurve, mint)
	if err != nil {
		logger.Error("Failed to derive associated bonding curve",
			zap.String("bonding_curve", bondingCurve.String()),
			zap.String("mint", mint.String()),
			zap.Error(err))
		return false, solana.PublicKey{}, fmt.Errorf("failed to derive associated bonding curve: %w", err)
	}

	// Step 3: Verify the associated curve exists and is properly owned
	acInfo, err := client.GetAccountInfo(ctx, associatedBondingCurve)
	if err != nil || acInfo == nil || acInfo.Value == nil {
		logger.Warn("Associated bonding curve does not exist or cannot be accessed",
			zap.String("associated_curve", associatedBondingCurve.String()),
			zap.Error(err))
		return false, solana.PublicKey{}, fmt.Errorf("associated bonding curve not found")
	}

	// Check ownership - associated token accounts are owned by the Token Program
	if !acInfo.Value.Owner.Equals(solana.TokenProgramID) {
		logger.Warn("Associated bonding curve has incorrect ownership",
			zap.String("associated_curve", associatedBondingCurve.String()),
			zap.String("owner", acInfo.Value.Owner.String()),
			zap.String("expected", solana.TokenProgramID.String()))
		return false, solana.PublicKey{}, fmt.Errorf("associated bonding curve has incorrect ownership")
	}

	// Token is eligible - both bonding curve and associated bonding curve are properly setup
	logger.Info("Token is eligible for Pump.fun operations",
		zap.String("mint", mint.String()),
		zap.String("bonding_curve", bondingCurve.String()),
		zap.String("associated_bonding_curve", associatedBondingCurve.String()))

	return true, associatedBondingCurve, nil
}
