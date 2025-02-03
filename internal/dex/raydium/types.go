// internal/dex/raydium/types.go
package raydium

import (
	"github.com/gagliardetto/solana-go"
)

// Basic constants for Raydium.
var (
	RaydiumProgramID = solana.MustPublicKeyFromBase58("RAYDIUM_V4_PROGRAM_ID_REPLACE")
	TokenProgramID   = solana.MustPublicKeyFromBase58("TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA")
)

// SwapDirection is a simple enum for swap direction.
type SwapDirection byte

const (
	SwapDirectionBaseToQuote SwapDirection = iota
	SwapDirectionQuoteToBase
)

// SwapParams stores the data needed to perform a swap.
type SwapParams struct {
	// Pool info
	PoolAddress  solana.PublicKey
	AmmAuthority solana.PublicKey
	BaseVault    solana.PublicKey
	QuoteVault   solana.PublicKey

	// Token info
	SourceMint solana.PublicKey
	TargetMint solana.PublicKey

	// User info
	UserPublicKey               solana.PublicKey
	PrivateKey                  *solana.PrivateKey
	UserSourceTokenAccount      solana.PublicKey
	UserDestinationTokenAccount solana.PublicKey

	// Amounts
	AmountInLamports uint64
	MinAmountOut     uint64
	Direction        SwapDirection

	// Priority and slippage
	ComputeUnits        uint32 // If needed
	PriorityFeeLamports uint64
	WaitForConfirmation bool
}

// SwapResult represents the outcome of a swap.
type SwapResult struct {
	Signature solana.Signature
	AmountIn  uint64
	AmountOut uint64
	// Add more fields if needed
}

// SnipeParams is for quick-buy scenario
type SnipeParams struct {
	TokenMint    solana.PublicKey // The newly listed token
	SourceMint   solana.PublicKey // e.g. USDC or wSOL
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
