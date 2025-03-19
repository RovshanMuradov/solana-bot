// =============================================
// File: internal/dex/pumpfun/config.go
// =============================================
package pumpfun

import (
	"fmt"

	"github.com/gagliardetto/solana-go"
	"go.uber.org/zap"
)

// Known PumpFun protocol addresses
var (
	// Program ID for Pump.fun protocol
	PumpFunProgramID = solana.MustPublicKeyFromBase58("6EF8rrecthR5Dkzon8Nwu78hRvfCKubJ14M5uBEwF6P")

	// Event authority for the Pump.fun protocol
	PumpFunEventAuth = solana.MustPublicKeyFromBase58("Ce6TQqeHC9p8KetsN6JsjHK7UTZk7nasjjnr7XxXp9F1")

	// Solana system accounts
	SysvarRentPubkey         = solana.MustPublicKeyFromBase58("SysvarRent111111111111111111111111111111111")
	AssociatedTokenProgramID = solana.MustPublicKeyFromBase58("ATokenGPvbdGVxr1b2hvZbsiqW5xWH25efTNsLJA8knL")
)

// PDA seeds used for account derivation
var (
	globalSeed          = []byte("global")
	bondingCurveSeed    = []byte("bonding-curve")
	associatedCurveSeed = []byte("associated-curve")

	// Version control for instruction discriminators
	DiscriminatorVersion = "v3" // Control which version to use
)

// Discriminator versions for Pump.fun instructions
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
func GetDefaultConfig() *Config {
	return &Config{
		GraduationThreshold: 100.0,
		AllowSellBeforeFull: true,
		MonitorInterval:     "5s",
		ContractAddress:     PumpFunProgramID, // Инициализируем из констант
		EventAuthority:      PumpFunEventAuth, // Инициализируем из констант
	}
}

// SetupForToken configures the Config instance for a specific token by
// deriving all necessary Pump.fun account addresses associated with the token.
//
// Parameters:
//   - tokenMint: The base58-encoded public key of the token mint
//   - logger: Zap logger instance for logging configuration details
//
// Returns an error if any address derivation fails or if inputs are invalid.
func (cfg *Config) SetupForToken(tokenMint string, logger *zap.Logger) error {
	// Validate and set token mint first
	if tokenMint == "" {
		return fmt.Errorf("token mint address is required")
	}

	var err error
	cfg.Mint, err = solana.PublicKeyFromBase58(tokenMint)
	if err != nil {
		return fmt.Errorf("invalid token mint address: %w", err)
	}

	// Set program ID if not already set
	if cfg.ContractAddress.IsZero() {
		cfg.ContractAddress = PumpFunProgramID
	}

	// Derive Global Account PDA
	var bump uint8
	cfg.Global, bump, err = solana.FindProgramAddress(
		[][]byte{globalSeed},
		cfg.ContractAddress,
	)
	if err != nil {
		return fmt.Errorf("failed to derive global account: %w", err)
	}

	logger.Debug("Derived global account",
		zap.String("address", cfg.Global.String()),
		zap.Uint8("bump", bump))

	// Set event authority address if not already set
	if cfg.EventAuthority.IsZero() {
		cfg.EventAuthority = PumpFunEventAuth
	}

	// Calculate bonding curve address
	var bondingCurveBump uint8
	cfg.BondingCurve, bondingCurveBump, err = deriveBondingCurveAddress(cfg.Mint, cfg.ContractAddress)
	if err != nil {
		return fmt.Errorf("failed to derive bonding curve: %w", err)
	}

	// Calculate associated bonding curve using direct FindAssociatedTokenAddress method
	// This is the key fix according to the technical specification
	cfg.AssociatedBondingCurve, _, err = solana.FindAssociatedTokenAddress(cfg.BondingCurve, cfg.Mint)
	if err != nil {
		return fmt.Errorf("failed to derive associated bonding curve: %w", err)
	}

	// Set fee recipient to a default address (usually derived from global account data)
	// This will be updated later with the actual value through UpdateFeeRecipient
	cfg.FeeRecipient = cfg.Global // Временно установим на Global, чтобы не было нулевого значения

	logger.Info("PumpFun configuration prepared",
		zap.String("program_id", cfg.ContractAddress.String()),
		zap.String("global_account", cfg.Global.String()),
		zap.String("token_mint", cfg.Mint.String()),
		zap.String("bonding_curve", cfg.BondingCurve.String()),
		zap.Uint8("bonding_curve_bump", bondingCurveBump),
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
// Returns the derived address, bump seed, and any error encountered
func deriveBondingCurveAddress(tokenMint, programID solana.PublicKey) (solana.PublicKey, uint8, error) {
	// Validate input parameters
	if tokenMint.IsZero() {
		return solana.PublicKey{}, 0, fmt.Errorf("tokenMint cannot be zero")
	}
	if programID.IsZero() {
		return solana.PublicKey{}, 0, fmt.Errorf("programID cannot be zero")
	}

	// Calculate the program derived address
	address, bump, err := solana.FindProgramAddress(
		[][]byte{bondingCurveSeed, tokenMint.Bytes()},
		programID,
	)
	if err != nil {
		return solana.PublicKey{}, 0, fmt.Errorf("failed to derive bonding curve address: %w", err)
	}

	return address, bump, nil
}

// Note: We've removed deriveAssociatedCurveAddress in favor of the direct solana.FindAssociatedTokenAddress
// method, which correctly implements the protocol's address derivation scheme
