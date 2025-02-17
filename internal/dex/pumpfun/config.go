// =============================================
// File: internal/dex/pumpfun/config.go
// =============================================
package pumpfun

import (
	"time"

	"github.com/gagliardetto/solana-go"
	"go.uber.org/zap"
)

// Config holds Pump.fun config params.
type Config struct {
	ContractAddress        solana.PublicKey
	GraduationThreshold    float64
	AllowSellBeforeFull    bool
	MonitorInterval        string
	MonitorDuration        func() time.Duration
	Global                 solana.PublicKey
	FeeRecipient           solana.PublicKey
	Mint                   solana.PublicKey
	BondingCurve           solana.PublicKey
	AssociatedBondingCurve solana.PublicKey
	EventAuthority         solana.PublicKey
}

// GetDefaultConfig returns default Pump.fun configuration.
func GetDefaultConfig(logger *zap.Logger) *Config {
	_ = logger                                       // используем, чтобы избежать ошибки unused-parameter
	validDummy := "11111111111111111111111111111111" // корректное Base58 значение
	return &Config{
		ContractAddress:        solana.MustPublicKeyFromBase58(validDummy),
		GraduationThreshold:    100.0,
		AllowSellBeforeFull:    true,
		MonitorInterval:        "5s",
		MonitorDuration:        func() time.Duration { return 5 * time.Second },
		Global:                 solana.MustPublicKeyFromBase58(validDummy),
		FeeRecipient:           solana.MustPublicKeyFromBase58(validDummy),
		Mint:                   solana.MustPublicKeyFromBase58(validDummy),
		BondingCurve:           solana.MustPublicKeyFromBase58(validDummy),
		AssociatedBondingCurve: solana.MustPublicKeyFromBase58(validDummy),
		EventAuthority:         solana.MustPublicKeyFromBase58(validDummy),
	}
}
