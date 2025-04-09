// =============================
// File: internal/dex/pumpfun/config.go
// =============================
package pumpfun

import (
	"fmt"

	"github.com/gagliardetto/solana-go"
	"go.uber.org/zap"
)

// Known PumpFun protocol addresses - содержит константы адресов протокола Pump.fun.
type Config struct {
	ContractAddress solana.PublicKey
	Global          solana.PublicKey
	FeeRecipient    solana.PublicKey
	EventAuthority  solana.PublicKey
	Mint            solana.PublicKey
	MonitorInterval string
}

// GetDefaultConfig создает конфигурацию по умолчанию для Pump.fun DEX.
func GetDefaultConfig() *Config {
	return &Config{
		ContractAddress: PumpFunProgramID,
		EventAuthority:  PumpFunEventAuth,
		MonitorInterval: "5s",
	}
}

// SetupForToken настраивает экземпляр Config для конкретного токена.
// Метод выполняет необходимую инициализацию и проверки для работы с
// указанным токеном в протоколе Pump.fun.
func (cfg *Config) SetupForToken(tokenMint string, logger *zap.Logger) error {
	// Шаг 1: Проверка наличия адреса минта токена
	// Адрес токена обязателен для работы с Pump.fun
	if tokenMint == "" {
		return fmt.Errorf("token mint address is required")
	}

	// Шаг 2: Конвертация строкового адреса в PublicKey
	// Преобразуем base58-строку в объект PublicKey
	var err error
	cfg.Mint, err = solana.PublicKeyFromBase58(tokenMint)
	if err != nil {
		// Шаг 3: Обработка ошибки при некорректном адресе
		return fmt.Errorf("invalid token mint address: %w", err)
	}

	// Шаг 4: Установка адреса программы, если он не указан
	// Если ContractAddress не инициализирован (нулевой адрес), используем стандартный
	if cfg.ContractAddress.IsZero() {
		cfg.ContractAddress = PumpFunProgramID
	}

	// Шаг 5: Вычисление адреса глобального аккаунта
	// Используем PDA (Program Derived Address) с seed "global"
	cfg.Global, _, err = solana.FindProgramAddress(
		[][]byte{[]byte("global")},
		cfg.ContractAddress,
	)
	if err != nil {
		// Шаг 6: Обработка ошибки при неудачном вычислении адреса
		return fmt.Errorf("failed to derive global account: %w", err)
	}

	// Шаг 7: Установка EventAuthority, если не указан
	// Если EventAuthority не инициализирован, используем стандартный
	if cfg.EventAuthority.IsZero() {
		cfg.EventAuthority = PumpFunEventAuth
	}

	// Шаг 8: Примечание о получателе комиссий
	// FeeRecipient будет установлен позже при загрузке глобального аккаунта

	// Шаг 9: Логирование успешной настройки
	logger.Info("PumpFun configuration prepared",
		zap.String("program_id", cfg.ContractAddress.String()),
		zap.String("global_account", cfg.Global.String()),
		zap.String("token_mint", cfg.Mint.String()),
		zap.String("event_authority", cfg.EventAuthority.String()))

	// Шаг 10: Возврат успешного результата
	return nil
}
