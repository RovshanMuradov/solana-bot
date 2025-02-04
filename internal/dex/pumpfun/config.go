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
	_ = logger // Искусственно «используем» logger, чтобы избежать ошибки unused-parameter.
	return &Config{
		ContractAddress:        solana.MustPublicKeyFromBase58("11111111111111111111111111111111"),
		GraduationThreshold:    100.0,
		AllowSellBeforeFull:    true,
		MonitorInterval:        "5s",
		MonitorDuration:        func() time.Duration { return 5 * time.Second },
		Global:                 solana.MustPublicKeyFromBase58("GLOBAL_ACCOUNT"),
		FeeRecipient:           solana.MustPublicKeyFromBase58("FEE_RECIPIENT"),
		Mint:                   solana.MustPublicKeyFromBase58("MINT_ACCOUNT"),
		BondingCurve:           solana.MustPublicKeyFromBase58("BONDING_CURVE_ACCOUNT"),
		AssociatedBondingCurve: solana.MustPublicKeyFromBase58("ASSOCIATED_BONDING_CURVE"),
		EventAuthority:         solana.MustPublicKeyFromBase58("EVENT_AUTHORITY"),
	}
}
