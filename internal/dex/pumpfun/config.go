// =============================================
// File: internal/dex/pumpfun/config.go
// =============================================
package pumpfun

import (
	"fmt"

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

// PDA seeds used for account derivation
var (
	globalSeed               = []byte("global")
	bondingCurveSeed         = []byte("bonding-curve")
	SysvarRentPubkey         = solana.MustPublicKeyFromBase58("SysvarRent111111111111111111111111111111111")
	AssociatedTokenProgramID = solana.MustPublicKeyFromBase58("ATokenGPvbdGVxr1b2hvZbsiqW5xWH25efTNsLJA8knL")

	// Version control for instruction discriminators
	DiscriminatorVersion = "v3" // Control which version to use
)

// Discriminator versions
var (
	// Buy discriminators directly from the Pump.fun SDK IDL
	BuyDiscriminators = map[string][]byte{
		"v3": {0x66, 0x06, 0x3d, 0x12, 0x01, 0xda, 0xeb, 0xea}, // Correct full discriminator
	}

	// Sell discriminators directly from the Pump.fun SDK IDL
	SellDiscriminators = map[string][]byte{
		"v3": {0x33, 0xe6, 0x85, 0xa4, 0x01, 0x7f, 0x83, 0xad}, // Correct full discriminator
	}
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
	MonitorInterval     string // Duration string for monitoring intervals
}

// GetDefaultConfig creates a new configuration with default values
func GetDefaultConfig(_ *zap.Logger) *Config {
	return &Config{
		GraduationThreshold: 100.0,
		AllowSellBeforeFull: true,
		MonitorInterval:     "5s",
	}
}

// SetupForToken configures the Config instance for a specific token
func (cfg *Config) SetupForToken(tokenMint string, logger *zap.Logger) error {
	var err error

	// Set program ID
	cfg.ContractAddress, err = solana.PublicKeyFromBase58(PumpFunProgramID)
	if err != nil {
		return fmt.Errorf("invalid program ID: %w", err)
	}

	// Derive Global Account PDA
	cfg.Global, _, err = solana.FindProgramAddress(
		[][]byte{globalSeed},
		cfg.ContractAddress,
	)
	if err != nil {
		return fmt.Errorf("failed to derive global account: %w", err)
	}

	// Set event authority address
	cfg.EventAuthority, err = solana.PublicKeyFromBase58(PumpFunEventAuth)
	if err != nil {
		return fmt.Errorf("invalid event authority: %w", err)
	}

	// Validate and set token mint
	if tokenMint == "" {
		return fmt.Errorf("token mint address is required")
	}

	cfg.Mint, err = solana.PublicKeyFromBase58(tokenMint)
	if err != nil {
		return fmt.Errorf("invalid token mint address: %w", err)
	}

	// Calculate bonding curve address
	cfg.BondingCurve = deriveBondingCurveAddress(cfg.Mint, cfg.ContractAddress)

	// Calculate associated bonding curve
	cfg.AssociatedBondingCurve = deriveAssociatedCurveAddress(cfg.BondingCurve, cfg.ContractAddress)

	logger.Info("PumpFun configuration prepared",
		zap.String("program_id", cfg.ContractAddress.String()),
		zap.String("global_account", cfg.Global.String()),
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
func deriveAssociatedCurveAddress(bondingCurve, programID solana.PublicKey) solana.PublicKey {
	// Use local associated curve seed for derivation
	localAssociatedCurveSeed := []byte("associated-curve")

	address, _, _ := solana.FindProgramAddress(
		[][]byte{localAssociatedCurveSeed, bondingCurve.Bytes()},
		programID,
	)
	return address
}
