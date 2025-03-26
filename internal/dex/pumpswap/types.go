// =============================
// File: internal/dex/pumpswap/types.go
// =============================
package pumpswap

import (
	"encoding/binary"
	"fmt"

	"github.com/gagliardetto/solana-go"
)

// Дискриминаторы аккаунтов, извлечённые из IDL.
var (
	GlobalConfigDiscriminator = []byte{149, 8, 156, 202, 160, 252, 176, 217}
	PoolDiscriminator         = []byte{241, 154, 109, 4, 17, 177, 109, 188}
)

// GlobalConfig представляет глобальную конфигурацию PumpSwap.
type GlobalConfig struct {
	Admin                  solana.PublicKey    // Админский публичный ключ
	LPFeeBasisPoints       uint64              // Комиссия LP в базисных пунктах (0.01%)
	ProtocolFeeBasisPoints uint64              // Комиссия протокола в базисных пунктах (0.01%)
	DisableFlags           uint8               // Флаги для отключения определённых функций
	ProtocolFeeRecipients  [8]solana.PublicKey // Адреса получателей комиссии протокола
}

const (
	DisableCreatePool = 1 << iota
	DisableDeposit
	DisableWithdraw
	DisableBuy
	DisableSell
)

// Pool представляет пул ликвидности в PumpSwap.
type Pool struct {
	PoolBump              uint8            // PDA bump
	Index                 uint16           // Индекс пула
	Creator               solana.PublicKey // Создатель пула
	BaseMint              solana.PublicKey // Базовый токен (обычно SOL)
	QuoteMint             solana.PublicKey // Квотный токен
	LPMint                solana.PublicKey // Токен пула ликвидности
	PoolBaseTokenAccount  solana.PublicKey // Аккаунт базового токена пула
	PoolQuoteTokenAccount solana.PublicKey // Аккаунт квотного токена пула
	LPSupply              uint64           // Циркулирующее количество LP токенов
}

// PoolInfo содержит информацию о состоянии пула ликвидности.
type PoolInfo struct {
	Address               solana.PublicKey // Адрес пула
	BaseMint              solana.PublicKey // Базовый токен
	QuoteMint             solana.PublicKey // Квотный токен
	BaseReserves          uint64           // Количество базовых токенов в пуле
	QuoteReserves         uint64           // Количество квотных токенов в пуле
	LPSupply              uint64           // Количество LP токенов
	FeesBasisPoints       uint64           // Комиссия LP в базисных пунктах
	ProtocolFeeBPS        uint64           // Комиссия протокола в базисных пунктах
	LPMint                solana.PublicKey // Токен пула ликвидности
	PoolBaseTokenAccount  solana.PublicKey // Аккаунт базового токена пула
	PoolQuoteTokenAccount solana.PublicKey // Аккаунт квотного токена пула
}

// SwapTokens возвращает новый объект PoolInfo, где базовый и квотный токены поменяны местами.
func (p *PoolInfo) SwapTokens() *PoolInfo {
	return &PoolInfo{
		Address:               p.Address,
		BaseMint:              p.QuoteMint,
		QuoteMint:             p.BaseMint,
		BaseReserves:          p.QuoteReserves,
		QuoteReserves:         p.BaseReserves,
		LPSupply:              p.LPSupply,
		FeesBasisPoints:       p.FeesBasisPoints,
		ProtocolFeeBPS:        p.ProtocolFeeBPS,
		LPMint:                p.LPMint,
		PoolBaseTokenAccount:  p.PoolQuoteTokenAccount,
		PoolQuoteTokenAccount: p.PoolBaseTokenAccount,
	}
}

// ParseGlobalConfig парсит данные аккаунта в структуру GlobalConfig.
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

	adminBytes := make([]byte, 32)
	copy(adminBytes, data[pos:pos+32])
	config.Admin = solana.PublicKeyFromBytes(adminBytes)
	pos += 32

	config.LPFeeBasisPoints = binary.LittleEndian.Uint64(data[pos : pos+8])
	pos += 8

	config.ProtocolFeeBasisPoints = binary.LittleEndian.Uint64(data[pos : pos+8])
	pos += 8

	config.DisableFlags = data[pos]
	pos++

	for i := 0; i < 8; i++ {
		recipientBytes := make([]byte, 32)
		copy(recipientBytes, data[pos:pos+32])
		config.ProtocolFeeRecipients[i] = solana.PublicKeyFromBytes(recipientBytes)
		pos += 32
	}

	return config, nil
}
