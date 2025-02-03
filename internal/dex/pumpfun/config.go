package pumpfun

import (
	"time"

	"github.com/gagliardetto/solana-go"
	"go.uber.org/zap"
)

// PumpfunConfig содержит конфигурационные параметры для Pump.fun.
type PumpfunConfig struct {
	// Адрес контракта Pump.fun, отвечающего за создание и продажу токена.
	ContractAddress solana.PublicKey
	// Порог для graduation (например, 85.0 или 100.0 %).
	GraduationThreshold float64
	// Разрешить продажу токена до 100% bonding curve.
	AllowSellBeforeFull bool
	// Интервал опроса состояния bonding curve (например, "5s").
	MonitorInterval string
	MonitorDuration func() (d time.Duration)
	// Дополнительные аккаунты, необходимые для создания, покупки и продажи токена.
	Global                 solana.PublicKey
	FeeRecipient           solana.PublicKey
	Mint                   solana.PublicKey
	BondingCurve           solana.PublicKey
	AssociatedBondingCurve solana.PublicKey
	EventAuthority         solana.PublicKey
}

// GetDefaultConfig возвращает конфигурацию для Pump.fun.
// Здесь можно подставить реальные адреса и параметры либо получать их из файла конфигурации.
func GetDefaultConfig(logger *zap.Logger) *PumpfunConfig {
	// Примечание: замените строки-заменители на реальные Base58-адреса!
	return &PumpfunConfig{
		ContractAddress:        solana.MustPublicKeyFromBase58("PUMP_FUN_CONTRACT_ADDRESS"),
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
