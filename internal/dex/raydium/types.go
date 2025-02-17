// ==================================
// File: internal/dex/raydium/types.go
// ==================================
package raydium

import (
	"github.com/gagliardetto/solana-go"
)

var (
	RaydiumProgramID = solana.MustPublicKeyFromBase58("675kPX9MHTjS2zt1qfr1NYHuzeLXfQM9H24wFSUt1Mp8")
	TokenProgramID   = solana.MustPublicKeyFromBase58("TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA")
)

type SwapDirection byte

const (
	SwapDirectionBaseToQuote SwapDirection = iota
	SwapDirectionQuoteToBase
)

type SwapParams struct {
	PoolAddress  solana.PublicKey
	AmmAuthority solana.PublicKey
	BaseVault    solana.PublicKey
	QuoteVault   solana.PublicKey

	SourceMint solana.PublicKey
	TargetMint solana.PublicKey

	UserPublicKey               solana.PublicKey
	PrivateKey                  *solana.PrivateKey
	UserSourceTokenAccount      solana.PublicKey
	UserDestinationTokenAccount solana.PublicKey

	AmountInLamports uint64
	MinAmountOut     uint64
	Direction        SwapDirection

	ComputeUnits        uint32
	PriorityFeeLamports uint64
	WaitForConfirmation bool
}

type SwapResult struct {
	Signature solana.Signature
	AmountIn  uint64
	AmountOut uint64
}

type SnipeParams struct {
	TokenMint    solana.PublicKey
	SourceMint   solana.PublicKey
	AmmAuthority solana.PublicKey
	BaseVault    solana.PublicKey
	QuoteVault   solana.PublicKey

	UserPublicKey solana.PublicKey
	PrivateKey    *solana.PrivateKey
	UserSourceATA solana.PublicKey
	UserDestATA   solana.PublicKey

	AmountInLamports    uint64
	MinOutLamports      uint64
	PriorityFeeLamports uint64
}
