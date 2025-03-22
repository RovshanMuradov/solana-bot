// =============================
// File: internal/dex/pumpswap/config.go
// =============================
package pumpswap

import (
	"fmt"

	"github.com/gagliardetto/solana-go"
	"go.uber.org/zap"
)

// Program identifiers and constants
var (
	// PumpSwapProgramID is the address of the PumpSwap program
	PumpSwapProgramID = solana.MustPublicKeyFromBase58("pAMMBay6oceH9fJKBRHGP5D4bD4sWpmSwMn52FMfXEA")

	// EventAuthority is the address of the event authority
	EventAuthority = solana.MustPublicKeyFromBase58("Ce6TQqeHC9p8KetsN6JsjHK7UTZk7nasjjnr7XxXp9F1")

	// TokenProgramID is the Solana token program ID
	TokenProgramID = solana.MustPublicKeyFromBase58("TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA")

	// SystemProgramID is the Solana system program ID
	SystemProgramID = solana.MustPublicKeyFromBase58("11111111111111111111111111111111")

	// AssociatedTokenProgramID is the Solana associated token program ID
	AssociatedTokenProgramID = solana.MustPublicKeyFromBase58("ATokenGPvbdGVxr1b2hvZbsiqW5xWH25efTNsLJA8knL")
)

// Config holds the configuration for PumpSwap DEX interactions
type Config struct {
	// Program addresses
	ProgramID      solana.PublicKey
	GlobalConfig   solana.PublicKey
	EventAuthority solana.PublicKey

	// Token configuration
	BaseMint  solana.PublicKey // SOL or stable coin
	QuoteMint solana.PublicKey // Token migrated from bonding curve

	// Pool parameters
	PoolAddress solana.PublicKey // Address of the discovered pool
	LPMint      solana.PublicKey // Liquidity pool token mint

	// Monitoring parameters
	MonitorInterval string
}

// GetDefaultConfig returns a default configuration for PumpSwap
func GetDefaultConfig() *Config {
	return &Config{
		ProgramID:       PumpSwapProgramID,
		EventAuthority:  EventAuthority,
		MonitorInterval: "5s",
	}
}

// SetupForToken configures the PumpSwap instance for a specific token
func (cfg *Config) SetupForToken(quoteTokenMint string, logger *zap.Logger) error {
	// Validate token mint address
	if quoteTokenMint == "" {
		return fmt.Errorf("token mint address is required")
	}

	var err error
	cfg.QuoteMint, err = solana.PublicKeyFromBase58(quoteTokenMint)
	if err != nil {
		return fmt.Errorf("invalid token mint address: %w", err)
	}

	// Set SOL as the base mint
	cfg.BaseMint = solana.MustPublicKeyFromBase58("So11111111111111111111111111111111111111112")

	// Derive the global config address
	globalConfigAddr, _, err := cfg.DeriveGlobalConfigAddress()
	if err != nil {
		return fmt.Errorf("failed to derive global config address: %w", err)
	}
	cfg.GlobalConfig = globalConfigAddr

	logger.Info("PumpSwap configuration prepared",
		zap.String("program_id", cfg.ProgramID.String()),
		zap.String("global_config", cfg.GlobalConfig.String()),
		zap.String("base_mint", cfg.BaseMint.String()),
		zap.String("quote_mint", cfg.QuoteMint.String()))

	return nil
}

// DeriveGlobalConfigAddress derives the PDA for the global config account
func (cfg *Config) DeriveGlobalConfigAddress() (solana.PublicKey, uint8, error) {
	return solana.FindProgramAddress(
		[][]byte{[]byte("global_config")},
		cfg.ProgramID,
	)
}

// DerivePoolAddress derives the PDA for a pool with specific parameters
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
