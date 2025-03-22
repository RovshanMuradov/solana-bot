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
	binary.BigEndian.PutUint64(data[8:16], baseAmountOut)

	// Add maxQuoteAmountIn parameter (u64 - 8 bytes)
	binary.BigEndian.PutUint64(data[16:24], maxQuoteAmountIn)

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
) *solana.GenericInstruction {
	// Create data buffer with discriminator and parameters
	data := make([]byte, 8+8+8) // 8 bytes discriminator + 8 bytes baseAmountIn + 8 bytes minQuoteAmountOut

	// Copy discriminator
	copy(data[0:8], sellDiscriminator)

	// Add baseAmountIn parameter (u64 - 8 bytes)
	binary.BigEndian.PutUint64(data[8:16], baseAmountIn)

	// Add minQuoteAmountOut parameter (u64 - 8 bytes)
	binary.BigEndian.PutUint64(data[16:24], minQuoteAmountOut)

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

// createAssociatedTokenAccountInstruction creates an instruction to create an associated token account
// func createAssociatedTokenAccountInstruction(
//	payer solana.PublicKey,
//	owner solana.PublicKey,
//	mint solana.PublicKey,
// ) *token.CreateAssociatedTokenAccount {
//	return token.NewCreateAssociatedTokenAccountInstruction(
//		payer, // Funding account
//		owner, // Account owner
//		mint,  // Token mint
//	).Build()
//}

// createComputeBudgetRequestHeapFrameInstruction creates a compute budget instruction to request more heap memory
// Unused function kept for future reference
/*
func createComputeBudgetRequestHeapFrameInstruction(bytes uint32) solana.Instruction {
	// Create program ID for compute budget program
	computeBudgetProgramID := solana.MustPublicKeyFromBase58("ComputeBudget111111111111111111111111111111")

	// Create data buffer with instruction index and bytes parameter
	data := make([]byte, 9) // 1 byte instruction index + 8 bytes bytes parameter

	// Instruction index 1 = RequestHeapFrame
	data[0] = 1

	// Add bytes parameter (u32 - 4 bytes)
	for i := 0; i < 4; i++ {
		data[i+1] = byte(bytes >> (i * 8))
	}

	return solana.NewInstruction(
		computeBudgetProgramID,
		[]*solana.AccountMeta{},
		data,
	)
}
*/

// createComputeBudgetRequestUnitsFeeInstruction creates a compute budget instruction to request compute units with priority fee
func createComputeBudgetRequestUnitsFeeInstruction(_ uint32, microLamports uint64) solana.Instruction {
	// Create program ID for compute budget program
	computeBudgetProgramID := solana.MustPublicKeyFromBase58("ComputeBudget111111111111111111111111111111")

	// Create data buffer with instruction index, units, and microLamports parameters
	data := make([]byte, 9) // 1 byte instruction index + 4 bytes units + 4 bytes microLamports

	// Instruction index 2 = SetComputeUnitPrice
	data[0] = 2

	// Add microLamports parameter (u64 - 8 bytes)
	for i := 0; i < 8; i++ {
		data[i+1] = byte(microLamports >> (i * 8))
	}

	return solana.NewInstruction(
		computeBudgetProgramID,
		[]*solana.AccountMeta{},
		data,
	)
}

// createComputeBudgetRequestUnitsInstruction creates a compute budget instruction to request compute units
func createComputeBudgetRequestUnitsInstruction(units uint32) solana.Instruction {
	// Create program ID for compute budget program
	computeBudgetProgramID := solana.MustPublicKeyFromBase58("ComputeBudget111111111111111111111111111111")

	// Create data buffer with instruction index and units parameter
	data := make([]byte, 5) // 1 byte instruction index + 4 bytes units

	// Instruction index 0 = RequestUnits
	data[0] = 0

	// Add units parameter (u32 - 4 bytes)
	for i := 0; i < 4; i++ {
		data[i+1] = byte(units >> (i * 8))
	}

	return solana.NewInstruction(
		computeBudgetProgramID,
		[]*solana.AccountMeta{},
		data,
	)
}

// createWrapSolInstruction creates an instruction to wrap SOL into wSOL
// func createWrapSolInstruction(
//	owner solana.PublicKey,
//	tokenAccount solana.PublicKey,
//	amount uint64,
// ) *system.Transfer {
//	return system.NewTransferInstruction(
//		amount,
//		owner,
//		tokenAccount,
//	).Build()
//}
//
// createUnwrapSolInstruction creates an instruction to unwrap wSOL back to SOL
// func createUnwrapSolInstruction(
//	owner solana.PublicKey,
//	tokenAccount solana.PublicKey,
// ) *token.CloseAccount {
//	return token.NewCloseAccountInstruction(
//		tokenAccount,
//		owner,
//		owner,
//		[]solana.PublicKey{owner},
//	).Build()
//}
