// =============================
// File: internal/dex/pumpfun/types.go
// =============================
package pumpfun

import (
	"github.com/gagliardetto/solana-go"
)

// GlobalAccount represents the structure of the PumpFun global account data
type GlobalAccount struct {
	Discriminator  [8]byte
	Initialized    bool
	Authority      solana.PublicKey
	FeeRecipient   solana.PublicKey
	FeeBasisPoints uint64
}

type BondingCurve struct {
	VirtualTokenReserves uint64
	VirtualSolReserves   uint64
	// Другие поля могут быть добавлены в зависимости от структуры аккаунта Pump.fun
}