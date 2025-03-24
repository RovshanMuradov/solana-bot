// =============================
// File: internal/dex/pumpswap/config.go (исправление)
// =============================
package pumpswap

import (
	"fmt"

	"github.com/gagliardetto/solana-go"
	"go.uber.org/zap"
)

var (
	// PumpSwapProgramID – адрес программы PumpSwap.
	PumpSwapProgramID = solana.MustPublicKeyFromBase58("pAMMBay6oceH9fJKBRHGP5D4bD4sWpmSwMn52FMfXEA")
	// SystemProgramID – ID системной программы Solana.
	SystemProgramID = solana.MustPublicKeyFromBase58("11111111111111111111111111111111")
	// TokenProgramID – ID программы токенов Solana.
	TokenProgramID = solana.MustPublicKeyFromBase58("TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA")
	// AssociatedTokenProgramID – ID ассоциированной токенной программы.
	AssociatedTokenProgramID = solana.MustPublicKeyFromBase58("ATokenGPvbdGVxr1b2hvZbsiqW5xWH25efTNsLJA8knL")
)

// Config хранит конфигурацию для взаимодействия с PumpSwap.
type Config struct {
	ProgramID      solana.PublicKey
	GlobalConfig   solana.PublicKey
	EventAuthority solana.PublicKey

	BaseMint  solana.PublicKey // SOL или стабильная монета
	QuoteMint solana.PublicKey // Токен, приобретаемый по bonding curve

	PoolAddress solana.PublicKey // Обнаруженный адрес пула
	LPMint      solana.PublicKey // Токен пула ликвидности

	MonitorInterval string
}

// GetDefaultConfig возвращает конфигурацию по умолчанию для PumpSwap.
func GetDefaultConfig() *Config {
	eventAuthority, _, err := solana.FindProgramAddress(
		[][]byte{[]byte("__event_authority")},
		PumpSwapProgramID,
	)
	if err != nil {
		fmt.Printf("Failed to derive event authority: %v\n", err)
		eventAuthority = solana.PublicKey{}
	}

	return &Config{
		ProgramID:       PumpSwapProgramID,
		EventAuthority:  eventAuthority,
		MonitorInterval: "5s",
	}
}

// SetupForToken настраивает экземпляр PumpSwap для определённого токена.
func (cfg *Config) SetupForToken(quoteTokenMint string, logger *zap.Logger) error {
	if quoteTokenMint == "" {
		return fmt.Errorf("token mint address is required")
	}

	var err error
	cfg.QuoteMint, err = solana.PublicKeyFromBase58(quoteTokenMint)
	if err != nil {
		return fmt.Errorf("invalid token mint address: %w", err)
	}

	cfg.BaseMint = solana.MustPublicKeyFromBase58("So11111111111111111111111111111111111111112")

	globalConfigAddr, _, err := cfg.DeriveGlobalConfigAddress()
	if err != nil {
		return fmt.Errorf("failed to derive global config address: %w", err)
	}
	cfg.GlobalConfig = globalConfigAddr

	if cfg.EventAuthority.IsZero() {
		cfg.EventAuthority, _, err = solana.FindProgramAddress(
			[][]byte{[]byte("__event_authority")},
			cfg.ProgramID,
		)
		if err != nil {
			return fmt.Errorf("failed to derive event authority: %w", err)
		}
	}

	logger.Info("PumpSwap configuration prepared",
		zap.String("program_id", cfg.ProgramID.String()),
		zap.String("global_config", cfg.GlobalConfig.String()),
		zap.String("base_mint", cfg.BaseMint.String()),
		zap.String("quote_mint", cfg.QuoteMint.String()),
		zap.String("event_authority", cfg.EventAuthority.String()))

	return nil
}

// DeriveGlobalConfigAddress вычисляет PDA для глобального аккаунта конфигурации.
func (cfg *Config) DeriveGlobalConfigAddress() (solana.PublicKey, uint8, error) {
	return solana.FindProgramAddress(
		[][]byte{[]byte("global_config")},
		cfg.ProgramID,
	)
}

// DerivePoolAddress вычисляет PDA для пула с заданными параметрами.
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
