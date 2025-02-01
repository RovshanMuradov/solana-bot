// internal/dex/pumpfun/types.go
package pumpfun

import "github.com/gagliardetto/solana-go"

// PumpfunToken содержит базовую информацию о токене, созданном через Pump.fun.
type PumpfunToken struct {
	Mint         solana.PublicKey // Mint токена
	BondingCurve solana.PublicKey // Аккаунт bonding curve
	Name         string
	Symbol       string
	MetadataURI  string
	CreatedAt    int64 // Unix timestamp
}
