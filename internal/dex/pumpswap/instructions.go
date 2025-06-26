// =============================
// File: internal/dex/pumpswap/instructions.go
// =============================
package pumpswap

import (
	"encoding/binary"
	"github.com/gagliardetto/solana-go"
)

// Instruction discriminators extracted from the IDL
var (
	buyDiscriminator  = []byte{102, 6, 61, 18, 1, 218, 235, 234}
	sellDiscriminator = []byte{51, 230, 133, 164, 1, 127, 131, 173}
)

// SwapInstructionParams contains all parameters needed to create a swap instruction
type SwapInstructionParams struct {
	// Operation type
	IsBuy bool

	// Account parameters
	PoolAddress                      solana.PublicKey
	User                             solana.PublicKey
	GlobalConfig                     solana.PublicKey
	BaseMint                         solana.PublicKey
	QuoteMint                        solana.PublicKey
	UserBaseTokenAccount             solana.PublicKey
	UserQuoteTokenAccount            solana.PublicKey
	PoolBaseTokenAccount             solana.PublicKey
	PoolQuoteTokenAccount            solana.PublicKey
	ProtocolFeeRecipient             solana.PublicKey
	ProtocolFeeRecipientTokenAccount solana.PublicKey
	BaseTokenProgram                 solana.PublicKey
	QuoteTokenProgram                solana.PublicKey
	EventAuthority                   solana.PublicKey
	ProgramID                        solana.PublicKey
	CoinCreatorVaultATA              solana.PublicKey
	CoinCreatorVaultAuthority        solana.PublicKey

	// Operation-specific parameters
	// For buy: Amount1 = baseAmountOut, Amount2 = maxQuoteAmountIn
	// For sell: Amount1 = baseAmountIn, Amount2 = minQuoteAmountOut
	Amount1 uint64
	Amount2 uint64
}

// createSwapInstruction creates an instruction to buy or sell tokens in PumpSwap
func createSwapInstruction(params *SwapInstructionParams) solana.Instruction {
	// Create data buffer with discriminator and parameters
	data := make([]byte, 8+8+8) // 8 bytes discriminator + 8 bytes amount1 + 8 bytes amount2

	// Choose discriminator based on operation type
	if params.IsBuy {
		copy(data[0:8], buyDiscriminator)
	} else {
		copy(data[0:8], sellDiscriminator)
	}

	// Add amount parameters
	binary.LittleEndian.PutUint64(data[8:16], params.Amount1)
	binary.LittleEndian.PutUint64(data[16:24], params.Amount2)

	// Create accounts list in the required order
	accountMetas := []*solana.AccountMeta{
		solana.NewAccountMeta(params.PoolAddress, false, false),
		solana.NewAccountMeta(params.User, true, true),
		solana.NewAccountMeta(params.GlobalConfig, false, false),
		solana.NewAccountMeta(params.BaseMint, false, false),
		solana.NewAccountMeta(params.QuoteMint, false, false),
		solana.NewAccountMeta(params.UserBaseTokenAccount, true, false),
		solana.NewAccountMeta(params.UserQuoteTokenAccount, true, false),
		solana.NewAccountMeta(params.PoolBaseTokenAccount, true, false),
		solana.NewAccountMeta(params.PoolQuoteTokenAccount, true, false),
		solana.NewAccountMeta(params.ProtocolFeeRecipient, false, false),
		solana.NewAccountMeta(params.ProtocolFeeRecipientTokenAccount, true, false),
		solana.NewAccountMeta(params.BaseTokenProgram, false, false),
		solana.NewAccountMeta(params.QuoteTokenProgram, false, false),
		solana.NewAccountMeta(SystemProgramID, false, false),
		solana.NewAccountMeta(AssociatedTokenProgramID, false, false),
		solana.NewAccountMeta(params.EventAuthority, false, false),
		solana.NewAccountMeta(params.ProgramID, false, false),
		solana.NewAccountMeta(params.CoinCreatorVaultATA, true, false),
		solana.NewAccountMeta(params.CoinCreatorVaultAuthority, false, false),
	}

	return solana.NewInstruction(params.ProgramID, accountMetas, data)
}
