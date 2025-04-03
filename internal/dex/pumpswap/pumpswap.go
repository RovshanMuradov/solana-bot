// =============================
// File: internal/dex/pumpswap/pumpswap.go
// =============================
package pumpswap

import (
	"context"
	"fmt"
	"github.com/gagliardetto/solana-go"
)

// effectiveMints возвращает эффективные значения базового и квотного минтов для свопа.
//
// Метод определяет корректный порядок токенов при выполнении операций свопа.
// Для операции свопа WSOL→токен базовый токен должен быть покупаемым токеном,
// а квотный – WSOL. Если в конфигурации указано обратное (base = WSOL, quote = токен),
// то метод инвертирует их порядок для обеспечения правильной логики свопа.
//
// Возвращает:
//   - baseMint: публичный ключ эффективного базового токена
//   - quoteMint: публичный ключ эффективного квотного токена
func (d *DEX) effectiveMints() (baseMint, quoteMint solana.PublicKey) {
	wsol := solana.SolMint
	// Если конфигурация указана как base = WSOL, а quote = токен,
	// то для свапа effectiveBaseMint = токен, effectiveQuoteMint = WSOL.
	if d.config.BaseMint.Equals(wsol) && !d.config.QuoteMint.Equals(wsol) {
		return d.config.QuoteMint, d.config.BaseMint
	}
	return d.config.BaseMint, d.config.QuoteMint
}

// getGlobalConfig получает глобальную конфигурацию программы PumpSwap с использованием кэша.
//
// Метод реализует потокобезопасное получение глобальной конфигурации с кэшированием.
// Сначала проверяется наличие конфигурации в кэше, и если она отсутствует,
// выполняется запрос к блокчейну с использованием двойной проверки с блокировкой (DCLP).
// Полученная конфигурация сохраняется в кэше для последующих запросов.
//
// Параметры:
//   - ctx: контекст выполнения операции
//
// Возвращает:
//   - *GlobalConfig: указатель на структуру с глобальной конфигурацией
//   - error: ошибка, если не удалось получить конфигурацию
func (d *DEX) getGlobalConfig(ctx context.Context) (*GlobalConfig, error) {
	d.configMutex.RLock()
	config := d.globalConfig
	d.configMutex.RUnlock()

	if config != nil {
		return config, nil
	}

	// Если кэша нет, получаем данные с блокировкой записи
	d.configMutex.Lock()
	defer d.configMutex.Unlock()

	// Повторная проверка после получения блокировки
	if d.globalConfig != nil {
		return d.globalConfig, nil
	}

	config, err := d.fetchGlobalConfigFromChain(ctx)
	if err != nil {
		return nil, err
	}

	d.globalConfig = config
	return config, nil
}

// fetchGlobalConfigFromChain получает глобальную конфигурацию напрямую из блокчейна.
//
// Метод выполняет запрос к блокчейну для получения данных аккаунта глобальной конфигурации.
// Сначала вычисляется адрес конфигурации, затем получаются данные аккаунта,
// которые парсятся в структуру GlobalConfig. Этот метод вызывается из getGlobalConfig
// при отсутствии конфигурации в кэше.
//
// Параметры:
//   - ctx: контекст выполнения операции
//
// Возвращает:
//   - *GlobalConfig: указатель на структуру с глобальной конфигурацией
//   - error: ошибка, если не удалось получить или распарсить конфигурацию
func (d *DEX) fetchGlobalConfigFromChain(ctx context.Context) (*GlobalConfig, error) {
	globalConfigAddr, _, err := d.config.DeriveGlobalConfigAddress()
	if err != nil {
		return nil, fmt.Errorf("failed to derive global config address: %w", err)
	}

	accountInfo, err := d.client.GetAccountInfo(ctx, globalConfigAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to get global config: %w", err)
	}

	if accountInfo == nil || accountInfo.Value == nil {
		return nil, fmt.Errorf("global config account not found")
	}

	return ParseGlobalConfig(accountInfo.Value.Data.GetBinary())
}
