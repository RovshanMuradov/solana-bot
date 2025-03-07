// =============================================
// File: internal/dex/pumpfun/config.go
// =============================================
package pumpfun

import (
	"fmt"
	"time"

	"github.com/gagliardetto/solana-go"
	"go.uber.org/zap"
)

// Known PumpFun protocol addresses (constants)
const (
	// Program ID - correct from SDK validation
	PumpFunProgramID = "6EF8rrecthR5Dkzon8Nwu78hRvfCKubJ14M5uBEwF6P"

	// Event authority from SDK - CORRECTED
	PumpFunEventAuth = "Ce6TQqeHC9p8KetsN6JsjHK7UTZk7nasjjnr7XxXp9F1"
)

// safePublicKeyFromBase58 validates and converts a base58 address string to a solana.PublicKey
func safePublicKeyFromBase58(address string) (solana.PublicKey, error) {
	// Check length in base58 format
	if len(address) < 32 || len(address) > 44 {
		return solana.PublicKey{}, fmt.Errorf("invalid address length: %d", len(address))
	}

	return solana.PublicKeyFromBase58(address)
}

// PDA seeds used for account derivation
var (
	globalSeed        = []byte("global")
	bondingCurveSeed  = []byte("bonding-curve")
	mintAuthoritySeed = []byte("mint-authority")
)

// Config holds all necessary PumpFun protocol parameters and addresses
type Config struct {
	// Protocol addresses
	ContractAddress solana.PublicKey
	Global          solana.PublicKey
	FeeRecipient    solana.PublicKey
	EventAuthority  solana.PublicKey

	// Token specific addresses
	Mint                   solana.PublicKey
	BondingCurve           solana.PublicKey
	AssociatedBondingCurve solana.PublicKey

	// Operational parameters
	GraduationThreshold float64
	AllowSellBeforeFull bool
	MonitorInterval     string
	MonitorDuration     func() time.Duration
}

// GetDefaultConfig creates a new configuration with default values
// The returned config must be updated with real values before use
func GetDefaultConfig(logger *zap.Logger) *Config {
	return &Config{
		// Default operational parameters
		GraduationThreshold: 100.0,
		AllowSellBeforeFull: true,
		MonitorInterval:     "5s",
		MonitorDuration:     func() time.Duration { return 5 * time.Second },
	}
}

// SetupForToken configures the Config instance for a specific token
// tokenMint is the address of the token to be traded on PumpFun
func (cfg *Config) SetupForToken(tokenMint string, logger *zap.Logger) error {
	var err error

	// Debug logging of all addresses
	logger.Debug("Debugging addresses before processing",
		zap.String("PumpFunProgramID", PumpFunProgramID),
		zap.Int("PumpFunProgramID_length", len(PumpFunProgramID)),
		zap.String("PumpFunEventAuth", PumpFunEventAuth),
		zap.String("tokenMint", tokenMint))

	// Set program ID
	cfg.ContractAddress, err = safePublicKeyFromBase58(PumpFunProgramID)
	if err != nil {
		logger.Error("Invalid program ID", zap.String("address", PumpFunProgramID), zap.Error(err))
		return fmt.Errorf("invalid program ID: %w", err)
	}

	// Derive Global Account PDA programmatically
	cfg.Global, _, err = solana.FindProgramAddress(
		[][]byte{globalSeed},
		cfg.ContractAddress,
	)
	if err != nil {
		logger.Error("Failed to derive global account PDA", zap.Error(err))
		return fmt.Errorf("failed to derive global account: %w", err)
	}

	logger.Info("Derived global account address",
		zap.String("global_account", cfg.Global.String()))

	// Set event authority address
	cfg.EventAuthority, err = safePublicKeyFromBase58(PumpFunEventAuth)
	if err != nil {
		logger.Error("Invalid event authority", zap.String("address", PumpFunEventAuth), zap.Error(err))
		return fmt.Errorf("invalid event authority: %w", err)
	}

	// Validate token mint
	if tokenMint == "" {
		return fmt.Errorf("token mint address is required")
	}

	// Parse and validate token mint address (using original address as is)
	cfg.Mint, err = safePublicKeyFromBase58(tokenMint)
	if err != nil {
		logger.Error("Invalid token mint address", zap.String("address", tokenMint), zap.Error(err))
		return fmt.Errorf("invalid token mint address: %w", err)
	}

	// Calculate bonding curve addresses using helper functions
	cfg.BondingCurve = deriveBondingCurveAddress(cfg.Mint, cfg.ContractAddress)

	// FIXED: Calculate associated bonding curve
	cfg.AssociatedBondingCurve = deriveAssociatedCurveAddress(cfg.BondingCurve, cfg.ContractAddress)

	// We will set FeeRecipient later after fetching global account data
	// It should be obtained from global account data

	logger.Info("PumpFun configuration prepared",
		zap.String("program_id", cfg.ContractAddress.String()),
		zap.String("global_account", cfg.Global.String()),
		zap.String("event_authority", cfg.EventAuthority.String()),
		zap.String("token_mint", cfg.Mint.String()),
		zap.String("bonding_curve", cfg.BondingCurve.String()),
		zap.String("associated_bonding_curve", cfg.AssociatedBondingCurve.String()))

	return nil
}

// UpdateFeeRecipient sets the fee recipient address after fetching it from global account data
func (cfg *Config) UpdateFeeRecipient(feeRecipient solana.PublicKey, logger *zap.Logger) {
	if feeRecipient.IsZero() {
		logger.Warn("Attempted to set zero fee recipient address")
		return
	}

	cfg.FeeRecipient = feeRecipient
	logger.Info("Updated fee recipient address", zap.String("fee_recipient", cfg.FeeRecipient.String()))
}

// deriveBondingCurveAddress calculates the PDA for a token's bonding curve
func deriveBondingCurveAddress(tokenMint, programID solana.PublicKey) solana.PublicKey {
	address, _, _ := solana.FindProgramAddress(
		[][]byte{bondingCurveSeed, tokenMint.Bytes()},
		programID,
	)
	return address
}

// deriveAssociatedCurveAddress calculates the PDA for the associated bonding curve
// Fixed to properly derive the PDA instead of returning empty key
func deriveAssociatedCurveAddress(bondingCurve, programID solana.PublicKey) solana.PublicKey {
	// In Pump.fun, the associated bonding curve is derived using the
	// bonding curve address as seed with a specific prefix
	associatedCurveSeed := []byte("associated-curve")

	address, _, _ := solana.FindProgramAddress(
		[][]byte{associatedCurveSeed, bondingCurve.Bytes()},
		programID,
	)
	return address
}
