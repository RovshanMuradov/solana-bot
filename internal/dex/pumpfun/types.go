// =============================
// File: internal/dex/pumpfun/types.go
// =============================
package pumpfun

import (
	"github.com/gagliardetto/solana-go"
)

// Тип BondingCurve (расширили под new fields)
type BondingCurve struct {
	VirtualTokenReserves uint64
	VirtualSolReserves   uint64
	RealTokenReserves    uint64           // новое
	RealSolReserves      uint64           // новое
	TokenTotalSupply     uint64           // новое
	Complete             bool             // новое
	Creator              solana.PublicKey // новое
}

// GlobalAccount (разбираем creator_fee_basis_points)
type GlobalAccount struct {
	Discriminator          [8]byte
	Initialized            bool
	Authority              solana.PublicKey
	FeeRecipient           solana.PublicKey
	FeeBasisPoints         uint64
	WithdrawAuthority      solana.PublicKey    // новое
	InitialVirtualTokenRes uint64              // новое
	InitialVirtualSolRes   uint64              // новое
	InitialRealTokenRes    uint64              // новое
	TokenTotalSupply       uint64              // новое
	EnableMigrate          bool                // новое
	PoolMigrationFee       uint64              // новое
	CreatorFeeBasisPoints  uint64              // новое
	FeeRecipients          [7]solana.PublicKey // новое
	SetCreatorAuthority    solana.PublicKey    // новое
}
