// =============================
// File: internal/dex/pumpswap/pumpswap.go
// =============================
package pumpswap

import (
	"context"
	"fmt"
	"github.com/gagliardetto/solana-go"
)

// effectiveMints возвращает эффективные значения базового и квотного минтов для свапа.
// Для операции swap WSOL→токен мы хотим, чтобы базовый токен был именно токеном (покупаемым),
// а квотный – WSOL. Если в конфигурации указано обратное (base = WSOL, quote = токен),
// то мы инвертируем их.
func (d *DEX) effectiveMints() (baseMint, quoteMint solana.PublicKey) {
	wsol := solana.SolMint
	// Если конфигурация указана как base = WSOL, а quote = токен,
	// то для свапа effectiveBaseMint = токен, effectiveQuoteMint = WSOL.
	if d.config.BaseMint.Equals(wsol) && !d.config.QuoteMint.Equals(wsol) {
		return d.config.QuoteMint, d.config.BaseMint
	}
	return d.config.BaseMint, d.config.QuoteMint
}

// В pumpswap.go:
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

// Выделение получения данных в отдельный метод
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
