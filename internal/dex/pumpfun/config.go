// internal/dex/pumpfun/config.go
package pumpfun

import (
	"encoding/csv"
	"fmt"
	"os"
	"time"

	"github.com/gagliardetto/solana-go"
	"go.uber.org/zap"
)

// PumpfunConfig содержит конфигурационные параметры для Pump.fun.
type Config struct {
	// Адрес контракта Pump.fun, отвечающего за покупку/продажу токена.
	ContractAddress solana.PublicKey
	// Порог для graduation (например, 100.0 означает 100%).
	GraduationThreshold float64
	// Разрешить продажу токена до 100% bonding curve.
	AllowSellBeforeFull bool
	// Интервал опроса состояния bonding curve (например, "5s").
	MonitorInterval string
	MonitorDuration func() time.Duration
	// Дополнительные аккаунты, необходимые для операций.
	Global                 solana.PublicKey
	FeeRecipient           solana.PublicKey
	Mint                   solana.PublicKey
	BondingCurve           solana.PublicKey
	AssociatedBondingCurve solana.PublicKey
	EventAuthority         solana.PublicKey
}

// GetTargetTokenFromCSV читает адрес целевого токена из CSV-файла tast.csv.
// Предполагается, что в первой строке файла содержится нужный Base58‑адрес.
func GetTargetTokenFromCSV(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open CSV file: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return "", fmt.Errorf("failed to read CSV file: %w", err)
	}
	if len(records) == 0 || len(records[0]) == 0 {
		return "", fmt.Errorf("CSV file is empty or missing target token")
	}
	return records[0][0], nil
}

// GetDefaultConfig возвращает конфигурацию для Pump.fun.
// Адрес контракта (target token) берётся из файла tast.csv.
func GetDefaultConfig(logger *zap.Logger) *Config {
	tokenContractStr, err := GetTargetTokenFromCSV("tast.csv")
	if err != nil {
		logger.Error("Error reading target token from CSV", zap.Error(err))
	}

	return &Config{
		ContractAddress:        solana.MustPublicKeyFromBase58(tokenContractStr),
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
