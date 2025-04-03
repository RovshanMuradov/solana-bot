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
// Эти константы определяют фиксированные адреса смарт-контрактов и аккаунтов,
// используемых протоколом Pump.fun на блокчейне Solana.

// Config содержит конфигурацию для взаимодействия с Pump.fun DEX.
// Эта структура хранит все необходимые адреса и параметры для
// выполнения операций в протоколе Pump.fun.
type Config struct {
	// Protocol addresses - адреса, связанные с протоколом
	ContractAddress solana.PublicKey // Адрес основной программы Pump.fun
	Global          solana.PublicKey // Адрес глобального аккаунта конфигурации
	FeeRecipient    solana.PublicKey // Адрес получателя комиссий
	EventAuthority  solana.PublicKey // Адрес авторити для событий

	// Token specific addresses - адреса, специфичные для конкретного токена
	Mint solana.PublicKey // Адрес минта токена, с которым работает DEX

	// Monitoring configuration - параметры мониторинга
	MonitorInterval string // Интервал обновления данных при мониторинге в формате длительности (например, "5s")
}

// GetDefaultConfig создает конфигурацию по умолчанию для Pump.fun DEX.
// Метод инициализирует структуру Config стандартными значениями, которые
// подходят для большинства случаев использования.
func GetDefaultConfig() *Config {
	// Шаг 1: Создание новой структуры Config с предустановленными значениями
	return &Config{
		// Шаг 2: Установка адреса программы Pump.fun из константы
		ContractAddress: PumpFunProgramID,

		// Шаг 3: Установка авторити для событий из константы
		EventAuthority: PumpFunEventAuth,

		// Шаг 4: Установка стандартного интервала мониторинга (5 секунд)
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
