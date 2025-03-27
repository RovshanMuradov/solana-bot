// =============================
// File: internal/dex/pumpswap/config.go
// =============================
package pumpswap

import (
	"encoding/binary"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"go.uber.org/zap"
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

func ParseGlobalConfig(data []byte) (*GlobalConfig, error) {
	if len(data) < 8 {
		return nil, fmt.Errorf("data too short for GlobalConfig")
	}

	for i := 0; i < 8; i++ {
		if data[i] != GlobalConfigDiscriminator[i] {
			return nil, fmt.Errorf("invalid discriminator for GlobalConfig")
		}
	}

	pos := 8

	if len(data) < pos+32+8+8+1+(32*8) {
		return nil, fmt.Errorf("data too short for GlobalConfig content")
	}

	config := &GlobalConfig{}

	config.Admin = solana.PublicKeyFromBytes(data[pos : pos+32])
	pos += 32

	config.LPFeeBasisPoints = binary.LittleEndian.Uint64(data[pos : pos+8])
	pos += 8

	config.ProtocolFeeBasisPoints = binary.LittleEndian.Uint64(data[pos : pos+8])
	pos += 8

	config.DisableFlags = data[pos]
	pos++

	for i := 0; i < 8; i++ {
		config.ProtocolFeeRecipients[i] = solana.PublicKeyFromBytes(data[pos : pos+32])
		pos += 32
	}

	return config, nil
}
