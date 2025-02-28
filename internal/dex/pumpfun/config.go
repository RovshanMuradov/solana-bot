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
	PumpFunProgramID  = "6EF8rrecthR5Dkzon8Nwu78hRvfCKubJ14M5uBEwF6P"
	PumpFunGlobalAcc  = "GLoB9vUEV7KgM8K9GH2HPDdUVW7Z5KjdwV9qeJuP5vMK"
	PumpFunFeeAccount = "GpuQAWofnm26GqJYJ5oMxt1G9nGP3wbpC7pJx8Ep9nYQ"
	PumpFunEventAuth  = "EventjEUSUW94QwJdGs7pPmxQEWzVHqRwjGZB5h9XxH3"
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
	bondingCurveSeed    = []byte("bonding-curve")
	associatedCurveSeed = []byte("associated-curve")
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
		zap.String("PumpFunGlobalAcc", PumpFunGlobalAcc),
		zap.String("PumpFunFeeAccount", PumpFunFeeAccount),
		zap.String("PumpFunEventAuth", PumpFunEventAuth),
		zap.String("tokenMint", tokenMint))

	// Set program ID
	programID := PumpFunProgramID
	cfg.ContractAddress, err = safePublicKeyFromBase58(programID)
	if err != nil {
		logger.Error("Invalid program ID", zap.String("address", programID), zap.Error(err))
		return fmt.Errorf("invalid program ID: %w", err)
	}

	// Set global account address
	cfg.Global, err = safePublicKeyFromBase58(PumpFunGlobalAcc)
	if err != nil {
		logger.Error("Invalid global account", zap.String("address", PumpFunGlobalAcc), zap.Error(err))
		return fmt.Errorf("invalid global account: %w", err)
	}

	// Set fee recipient address
	cfg.FeeRecipient, err = safePublicKeyFromBase58(PumpFunFeeAccount)
	if err != nil {
		logger.Error("Invalid fee account", zap.String("address", PumpFunFeeAccount), zap.Error(err))
		return fmt.Errorf("invalid fee account: %w", err)
	}

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

	// Calculate bonding curve addresses
	cfg.BondingCurve = deriveBondingCurveAddress(cfg.Mint, cfg.ContractAddress)
	cfg.AssociatedBondingCurve = deriveAssociatedCurveAddress(cfg.BondingCurve, cfg.ContractAddress)

	logger.Info("PumpFun configuration prepared",
		zap.String("token_mint", cfg.Mint.String()),
		zap.String("bonding_curve", cfg.BondingCurve.String()),
		zap.String("associated_curve", cfg.AssociatedBondingCurve.String()))

	return nil
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
	address, _, _ := solana.FindProgramAddress(
		[][]byte{associatedCurveSeed, bondingCurve.Bytes()},
		programID,
	)
	return address
}
