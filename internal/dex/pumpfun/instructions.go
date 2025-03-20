// =============================
// File: internal/dex/pumpfun/instructions.go
// =============================
package pumpfun

import (
	"encoding/binary"

	"github.com/gagliardetto/solana-go"
)

// Constants for the Pump.fun protocol
var (
	// Fixed discriminators for buy and sell functions
	buyDiscriminator  = []byte{0x66, 0x06, 0x3d, 0x12, 0x01, 0xda, 0xeb, 0xea}
	sellDiscriminator = []byte{0x33, 0xe6, 0x85, 0xa4, 0x01, 0x7f, 0x83, 0xad}
	
	// Program ID for exact-sol operations
	PumpFunExactSolProgramID = solana.MustPublicKeyFromBase58("6sbiyZ7mLKmYkES2AKYPHtg4FjQMaqVx3jTHez6ZtfmX")
)

// createAssociatedTokenAccountIdempotentInstruction creates an instruction to create an associated token account
func createAssociatedTokenAccountIdempotentInstruction(payer, wallet, mint solana.PublicKey) solana.Instruction {
	associatedTokenProgramID := solana.MustPublicKeyFromBase58("ATokenGPvbdGVxr1b2hvZbsiqW5xWH25efTNsLJA8knL")

	// Calculate the associated token account address
	ata, _, _ := solana.FindAssociatedTokenAddress(wallet, mint)

	return solana.NewInstruction(
		associatedTokenProgramID,
		[]*solana.AccountMeta{
			{PublicKey: payer, IsWritable: true, IsSigner: true},
			{PublicKey: ata, IsWritable: true, IsSigner: false},
			{PublicKey: wallet, IsWritable: false, IsSigner: false},
			{PublicKey: mint, IsWritable: false, IsSigner: false},
			{PublicKey: solana.SystemProgramID, IsWritable: false, IsSigner: false},
			{PublicKey: solana.TokenProgramID, IsWritable: false, IsSigner: false},
			{PublicKey: solana.SysVarRentPubkey, IsWritable: false, IsSigner: false},
		},
		[]byte{1}, // Instruction code 1 for create idempotent
	)
}

// createBuyInstruction creates a buy instruction for the Pump.fun protocol
func createBuyInstruction(
	programID,
	global,
	feeRecipient,
	mint,
	bondingCurve,
	associatedBondingCurve,
	userATA,
	userWallet,
	eventAuthority solana.PublicKey,
	amount,
	maxSolCost uint64,
) solana.Instruction {
	// Create instruction data with precise byte layout:
	// 1. 8-byte discriminator prefix
	// 2. 8-byte little-endian encoded amount
	// 3. 8-byte little-endian encoded maxSolCost
	data := make([]byte, 24)

	// Copy discriminator (8 bytes)
	copy(data[0:8], buyDiscriminator)

	// Add amount in little-endian bytes (8 bytes)
	binary.LittleEndian.PutUint64(data[8:16], amount)

	// Add max SOL cost in little-endian bytes (8 bytes)
	binary.LittleEndian.PutUint64(data[16:24], maxSolCost)

	// Account list MUST follow exact protocol-mandated order
	accounts := []*solana.AccountMeta{
		{PublicKey: global, IsSigner: false, IsWritable: false},
		{PublicKey: feeRecipient, IsSigner: false, IsWritable: true},
		{PublicKey: mint, IsSigner: false, IsWritable: false},
		{PublicKey: bondingCurve, IsSigner: false, IsWritable: true},
		{PublicKey: associatedBondingCurve, IsSigner: false, IsWritable: true},
		{PublicKey: userATA, IsSigner: false, IsWritable: true},
		{PublicKey: userWallet, IsSigner: true, IsWritable: true},
		{PublicKey: solana.SystemProgramID, IsSigner: false, IsWritable: false},
		{PublicKey: solana.TokenProgramID, IsSigner: false, IsWritable: false},
		{PublicKey: solana.SysVarRentPubkey, IsSigner: false, IsWritable: false},
		{PublicKey: eventAuthority, IsSigner: false, IsWritable: false},
		{PublicKey: programID, IsSigner: false, IsWritable: false},
	}

	return solana.NewInstruction(programID, accounts, data)
}

// createBuyExactSolInstruction creates an instruction for buying with an exact SOL amount
func createBuyExactSolInstruction(
	global,
	feeRecipient,
	mint,
	bondingCurve,
	associatedBondingCurve,
	userATA,
	userWallet,
	eventAuthority solana.PublicKey,
	solAmountLamports uint64,
) solana.Instruction {
	// Create instruction data - only 8 bytes for sol amount in lamports
	data := make([]byte, 8)
	binary.LittleEndian.PutUint64(data, solAmountLamports)

	// Account list follows the same order as regular buy instruction
	accounts := []*solana.AccountMeta{
		{PublicKey: global, IsSigner: false, IsWritable: false},
		{PublicKey: feeRecipient, IsSigner: false, IsWritable: true},
		{PublicKey: mint, IsSigner: false, IsWritable: false},
		{PublicKey: bondingCurve, IsSigner: false, IsWritable: true},
		{PublicKey: associatedBondingCurve, IsSigner: false, IsWritable: true},
		{PublicKey: userATA, IsSigner: false, IsWritable: true},
		{PublicKey: userWallet, IsSigner: true, IsWritable: true},
		{PublicKey: solana.SystemProgramID, IsSigner: false, IsWritable: false},
		{PublicKey: solana.TokenProgramID, IsSigner: false, IsWritable: false},
		{PublicKey: solana.SysVarRentPubkey, IsSigner: false, IsWritable: false},
		{PublicKey: eventAuthority, IsSigner: false, IsWritable: false},
		{PublicKey: PumpFunProgramID, IsSigner: false, IsWritable: false},
	}

	return solana.NewInstruction(PumpFunExactSolProgramID, accounts, data)
}

// createSellInstruction creates a sell instruction for the Pump.fun protocol
func createSellInstruction(
	programID,
	global,
	feeRecipient,
	mint,
	bondingCurve,
	associatedBondingCurve,
	userATA,
	userWallet,
	eventAuthority solana.PublicKey,
	amount,
	minSolOutput uint64,
) solana.Instruction {
	// Create instruction data with precise byte layout:
	// 1. 8-byte discriminator prefix
	// 2. 8-byte little-endian encoded amount
	// 3. 8-byte little-endian encoded minSolOutput
	data := make([]byte, 24)

	// Copy discriminator (8 bytes)
	copy(data[0:8], sellDiscriminator)

	// Add amount in little-endian bytes (8 bytes)
	binary.LittleEndian.PutUint64(data[8:16], amount)

	// Add min SOL output in little-endian bytes (8 bytes)
	binary.LittleEndian.PutUint64(data[16:24], minSolOutput)

	// Account list MUST follow exact protocol-mandated order
	accounts := []*solana.AccountMeta{
		{PublicKey: global, IsSigner: false, IsWritable: false},
		{PublicKey: feeRecipient, IsSigner: false, IsWritable: true},
		{PublicKey: mint, IsSigner: false, IsWritable: false},
		{PublicKey: bondingCurve, IsSigner: false, IsWritable: true},
		{PublicKey: associatedBondingCurve, IsSigner: false, IsWritable: true},
		{PublicKey: userATA, IsSigner: false, IsWritable: true},
		{PublicKey: userWallet, IsSigner: true, IsWritable: true},
		{PublicKey: solana.SystemProgramID, IsSigner: false, IsWritable: false},
		{PublicKey: solana.TokenProgramID, IsSigner: false, IsWritable: false},
		{PublicKey: solana.SysVarRentPubkey, IsSigner: false, IsWritable: false},
		{PublicKey: eventAuthority, IsSigner: false, IsWritable: false},
		{PublicKey: programID, IsSigner: false, IsWritable: false},
	}

	return solana.NewInstruction(programID, accounts, data)
}
