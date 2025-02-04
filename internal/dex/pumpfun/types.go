// ===================================
// File: internal/dex/pumpfun/types.go
// ===================================
package pumpfun

import "github.com/gagliardetto/solana-go"

// Token is a basic Pump.fun token info.
type Token struct {
	Mint         solana.PublicKey
	BondingCurve solana.PublicKey
	Name         string
	Symbol       string
	MetadataURI  string
	CreatedAt    int64
}
