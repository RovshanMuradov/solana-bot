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

// createBuyInstruction creates an instruction to buy tokens in PumpSwap
func createBuyInstruction(
	poolAddress solana.PublicKey,
	user solana.PublicKey,
	globalConfig solana.PublicKey,
	baseMint solana.PublicKey,
	quoteMint solana.PublicKey,
	userBaseTokenAccount solana.PublicKey,
	userQuoteTokenAccount solana.PublicKey,
	poolBaseTokenAccount solana.PublicKey,
	poolQuoteTokenAccount solana.PublicKey,
	protocolFeeRecipient solana.PublicKey,
	protocolFeeRecipientTokenAccount solana.PublicKey,
	baseTokenProgram solana.PublicKey,
	quoteTokenProgram solana.PublicKey,
	eventAuthority solana.PublicKey,
	programID solana.PublicKey,
	baseAmountOut uint64,
	maxQuoteAmountIn uint64,
) solana.Instruction {
	// Create data buffer with discriminator and parameters
	data := make([]byte, 8+8+8) // 8 bytes discriminator + 8 bytes baseAmountOut + 8 bytes maxQuoteAmountIn

	// Copy discriminator
	copy(data[0:8], buyDiscriminator)

	// Add baseAmountOut parameter (u64 - 8 bytes)
	binary.LittleEndian.PutUint64(data[8:16], baseAmountOut)

	// Add maxQuoteAmountIn parameter (u64 - 8 bytes)
	binary.LittleEndian.PutUint64(data[16:24], maxQuoteAmountIn)

	// Create accounts list in the required order from IDL
	accountMetas := []*solana.AccountMeta{
		{PublicKey: poolAddress, IsSigner: false, IsWritable: false},
		{PublicKey: user, IsSigner: true, IsWritable: true},
		{PublicKey: globalConfig, IsSigner: false, IsWritable: false},
		{PublicKey: baseMint, IsSigner: false, IsWritable: false},
		{PublicKey: quoteMint, IsSigner: false, IsWritable: false},
		{PublicKey: userBaseTokenAccount, IsSigner: false, IsWritable: true},
		{PublicKey: userQuoteTokenAccount, IsSigner: false, IsWritable: true},
		{PublicKey: poolBaseTokenAccount, IsSigner: false, IsWritable: true},
		{PublicKey: poolQuoteTokenAccount, IsSigner: false, IsWritable: true},
		{PublicKey: protocolFeeRecipient, IsSigner: false, IsWritable: false},
		{PublicKey: protocolFeeRecipientTokenAccount, IsSigner: false, IsWritable: true},
		{PublicKey: baseTokenProgram, IsSigner: false, IsWritable: false},
		{PublicKey: quoteTokenProgram, IsSigner: false, IsWritable: false},
		{PublicKey: SystemProgramID, IsSigner: false, IsWritable: false},
		{PublicKey: AssociatedTokenProgramID, IsSigner: false, IsWritable: false},
		{PublicKey: eventAuthority, IsSigner: false, IsWritable: false},
		{PublicKey: programID, IsSigner: false, IsWritable: false},
	}

	return solana.NewInstruction(programID, accountMetas, data)
}

// createSellInstruction creates an instruction to sell tokens in PumpSwap
func createSellInstruction(
	poolAddress solana.PublicKey,
	user solana.PublicKey,
	globalConfig solana.PublicKey,
	baseMint solana.PublicKey,
	quoteMint solana.PublicKey,
	userBaseTokenAccount solana.PublicKey,
	userQuoteTokenAccount solana.PublicKey,
	poolBaseTokenAccount solana.PublicKey,
	poolQuoteTokenAccount solana.PublicKey,
	protocolFeeRecipient solana.PublicKey,
	protocolFeeRecipientTokenAccount solana.PublicKey,
	baseTokenProgram solana.PublicKey,
	quoteTokenProgram solana.PublicKey,
	eventAuthority solana.PublicKey,
	programID solana.PublicKey,
	baseAmountIn uint64,
	minQuoteAmountOut uint64,
) solana.Instruction {
	// Create data buffer with discriminator and parameters
	data := make([]byte, 8+8+8) // 8 bytes discriminator + 8 bytes baseAmountIn + 8 bytes minQuoteAmountOut

	// Copy discriminator
	copy(data[0:8], sellDiscriminator)

	// Add baseAmountIn parameter (u64 - 8 bytes)
	binary.LittleEndian.PutUint64(data[8:16], baseAmountIn)

	// Add minQuoteAmountOut parameter (u64 - 8 bytes)
	binary.LittleEndian.PutUint64(data[16:24], minQuoteAmountOut)

	// Create accounts list in the required order from IDL
	accountMetas := []*solana.AccountMeta{
		{PublicKey: poolAddress, IsSigner: false, IsWritable: false},
		{PublicKey: user, IsSigner: true, IsWritable: true},
		{PublicKey: globalConfig, IsSigner: false, IsWritable: false},
		{PublicKey: baseMint, IsSigner: false, IsWritable: false},
		{PublicKey: quoteMint, IsSigner: false, IsWritable: false},
		{PublicKey: userBaseTokenAccount, IsSigner: false, IsWritable: true},
		{PublicKey: userQuoteTokenAccount, IsSigner: false, IsWritable: true},
		{PublicKey: poolBaseTokenAccount, IsSigner: false, IsWritable: true},
		{PublicKey: poolQuoteTokenAccount, IsSigner: false, IsWritable: true},
		{PublicKey: protocolFeeRecipient, IsSigner: false, IsWritable: false},
		{PublicKey: protocolFeeRecipientTokenAccount, IsSigner: false, IsWritable: true},
		{PublicKey: baseTokenProgram, IsSigner: false, IsWritable: false},
		{PublicKey: quoteTokenProgram, IsSigner: false, IsWritable: false},
		{PublicKey: SystemProgramID, IsSigner: false, IsWritable: false},
		{PublicKey: AssociatedTokenProgramID, IsSigner: false, IsWritable: false},
		{PublicKey: eventAuthority, IsSigner: false, IsWritable: false},
		{PublicKey: programID, IsSigner: false, IsWritable: false},
	}

	return solana.NewInstruction(programID, accountMetas, data)
}
