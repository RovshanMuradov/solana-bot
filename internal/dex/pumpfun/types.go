// ===================================
// File: internal/dex/pumpfun/types.go
// ===================================
package pumpfun

import (
	"time"

	"github.com/gagliardetto/solana-go"
)

// Token is a basic Pump.fun token info.
type Token struct {
	Mint         solana.PublicKey
	BondingCurve solana.PublicKey
	Name         string
	Symbol       string
	MetadataURI  string
	CreatedAt    int64
}

// BondingCurveInfo holds bonding curve state data.
type BondingCurveInfo struct {
	Progress    float64
	TotalSOL    float64
	MarketCap   float64
	LastUpdated time.Time
}

// GlobalAccount represents the structure of the PumpFun global account data
type GlobalAccount struct {
	Discriminator               [8]byte
	Initialized                 bool
	Authority                   solana.PublicKey
	FeeRecipient                solana.PublicKey
	InitialVirtualTokenReserves uint64
	InitialVirtualSolReserves   uint64
	InitialRealTokenReserves    uint64
	TokenTotalSupply            uint64
	FeeBasisPoints              uint64
}

// BuyInstructionAccounts holds account references for buy operation
type BuyInstructionAccounts struct {
	Global                 solana.PublicKey
	FeeRecipient           solana.PublicKey
	Mint                   solana.PublicKey
	BondingCurve           solana.PublicKey
	AssociatedBondingCurve solana.PublicKey
	EventAuthority         solana.PublicKey
	Program                solana.PublicKey
}

// SellInstructionAccounts holds account references for sell operation
type SellInstructionAccounts struct {
	Global                 solana.PublicKey
	FeeRecipient           solana.PublicKey
	Mint                   solana.PublicKey
	BondingCurve           solana.PublicKey
	AssociatedBondingCurve solana.PublicKey
	EventAuthority         solana.PublicKey
	Program                solana.PublicKey
}

// ProgramState represents the state of the PumpFun program and its key accounts
type ProgramState struct {
	ProgramID                  string `json:"program_id"`
	IsExecutable               bool   `json:"is_executable"`
	GlobalAccount              string `json:"global_account"`
	GlobalInitialized          bool   `json:"global_initialized"`
	GlobalOwner                string `json:"global_owner,omitempty"`
	BondingCurve               string `json:"bonding_curve"`
	BondingCurveInitialized    bool   `json:"bonding_curve_initialized"`
	BondingCurveOwner          string `json:"bonding_curve_owner,omitempty"`
	AssociatedCurve            string `json:"associated_curve,omitempty"`
	AssociatedCurveInitialized bool   `json:"associated_curve_initialized"`
	AssociatedCurveOwner       string `json:"associated_curve_owner,omitempty"`
	TokenMint                  string `json:"token_mint"`
	Error                      string `json:"error,omitempty"`
}

// Note: IsReady is implemented in checker.go
