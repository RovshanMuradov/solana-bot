// =============================
// File: internal/dex/pumpfun/config.go
// =============================
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
)

// Config holds the configuration for the Pump.fun DEX
type Config struct {
	// Protocol addresses
	ContractAddress solana.PublicKey
	Global          solana.PublicKey
	FeeRecipient    solana.PublicKey
	EventAuthority  solana.PublicKey

	// Token specific addresses
	Mint solana.PublicKey

	// Monitoring configuration
	MonitorInterval string // Duration string for monitoring intervals
}

// GetDefaultConfig creates a default configuration for the Pump.fun DEX
func GetDefaultConfig() *Config {
	return &Config{
		ContractAddress: PumpFunProgramID,
		EventAuthority:  PumpFunEventAuth,
		MonitorInterval: "5s",
	}
}

// SetupForToken configures the Config instance for a specific token
func (cfg *Config) SetupForToken(tokenMint string, logger *zap.Logger) error {
	// Validate and set token mint
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
	cfg.Global, _, err = solana.FindProgramAddress(
		[][]byte{[]byte("global")},
		cfg.ContractAddress,
	)
	if err != nil {
		return fmt.Errorf("failed to derive global account: %w", err)
	}

	// Set event authority if not already set
	if cfg.EventAuthority.IsZero() {
		cfg.EventAuthority = PumpFunEventAuth
	}

	// The fee recipient will be set later when fetching the global account

	logger.Info("PumpFun configuration prepared",
		zap.String("program_id", cfg.ContractAddress.String()),
		zap.String("global_account", cfg.Global.String()),
		zap.String("token_mint", cfg.Mint.String()),
		zap.String("event_authority", cfg.EventAuthority.String()))

	return nil
}
