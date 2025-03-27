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
	wsol := solana.MustPublicKeyFromBase58(WSOLMint)
	// Если конфигурация указана как base = WSOL, а quote = токен,
	// то для свапа effectiveBaseMint = токен, effectiveQuoteMint = WSOL.
	if d.config.BaseMint.Equals(wsol) && !d.config.QuoteMint.Equals(wsol) {
		return d.config.QuoteMint, d.config.BaseMint
	}
	return d.config.BaseMint, d.config.QuoteMint
}

func (d *DEX) getGlobalConfig(ctx context.Context) (*GlobalConfig, error) {
	// First check with read lock
	d.configMutex.RLock()
	if d.globalConfig != nil {
		config := d.globalConfig
		d.configMutex.RUnlock()
		return config, nil
	}
	d.configMutex.RUnlock()

	// If not found, use write lock for the entire fetch-and-set operation
	d.configMutex.Lock()
	defer d.configMutex.Unlock()

	// Double-check after acquiring write lock
	if d.globalConfig != nil {
		return d.globalConfig, nil
	}

	globalConfigAddr, _, err := d.config.DeriveGlobalConfigAddress()
	if err != nil {
		return nil, fmt.Errorf("failed to derive global config address: %w", err)
	}

	globalConfigInfo, err := d.client.GetAccountInfo(ctx, globalConfigAddr)
	if err != nil || globalConfigInfo == nil || globalConfigInfo.Value == nil {
		return nil, fmt.Errorf("failed to get global config: %w", err)
	}

	config, err := ParseGlobalConfig(globalConfigInfo.Value.Data.GetBinary())
	if err != nil {
		return nil, err
	}

	d.globalConfig = config
	return config, nil
}
