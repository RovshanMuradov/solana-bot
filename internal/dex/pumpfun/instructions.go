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
	sellDiscriminator = []byte{0x33, 0xe6, 0x85, 0xa4, 0x01, 0x7f, 0x83, 0xad}

	// Program ID for exact-sol operations
	PumpFunExactSolProgramID = solana.MustPublicKeyFromBase58("6sbiyZ7mLKmYkES2AKYPHtg4FjQMaqVx3jTHez6ZtfmX")
	AssociatedTokenProgramID = solana.SPLAssociatedTokenAccountProgramID
	SystemProgramID          = solana.SystemProgramID
	TokenProgramID           = solana.TokenProgramID
	SysVarRentPubkey         = solana.SysVarRentPubkey
)

//// PumpFunInstructionParams содержит все параметры для создания инструкций Pump.fun
//type PumpFunInstructionParams struct {
//	// Общие параметры
//	Global                 solana.PublicKey
//	FeeRecipient           solana.PublicKey
//	Mint                   solana.PublicKey
//	BondingCurve           solana.PublicKey
//	AssociatedBondingCurve solana.PublicKey
//	UserATA                solana.PublicKey
//	UserWallet             solana.PublicKey
//	EventAuthority         solana.PublicKey
//	ProgramID              solana.PublicKey
//
//	// Параметры операций
//	SolAmountLamports uint64 // Для buy exact SOL
//	TokenAmount       uint64 // Для sell
//	MinSolOutput      uint64 // Для sell
//}

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
		solana.NewAccountMeta(global, false, false),
		solana.NewAccountMeta(feeRecipient, true, false),
		solana.NewAccountMeta(mint, false, false),
		solana.NewAccountMeta(bondingCurve, true, false),
		solana.NewAccountMeta(associatedBondingCurve, true, false),
		solana.NewAccountMeta(userATA, true, false),
		solana.NewAccountMeta(userWallet, true, true),
		solana.NewAccountMeta(SystemProgramID, false, false),
		solana.NewAccountMeta(TokenProgramID, false, false),
		solana.NewAccountMeta(SysVarRentPubkey, false, false),
		solana.NewAccountMeta(eventAuthority, false, false),
		solana.NewAccountMeta(PumpFunProgramID, false, false),
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
		solana.NewAccountMeta(global, false, false),
		solana.NewAccountMeta(feeRecipient, true, false),
		solana.NewAccountMeta(mint, false, false),
		solana.NewAccountMeta(bondingCurve, true, false),
		solana.NewAccountMeta(associatedBondingCurve, true, false),
		solana.NewAccountMeta(userATA, true, false),
		solana.NewAccountMeta(userWallet, true, true),
		solana.NewAccountMeta(SystemProgramID, false, false),
		solana.NewAccountMeta(AssociatedTokenProgramID, false, false), // Сначала Associated Token Program
		solana.NewAccountMeta(TokenProgramID, false, false),           // Затем Token Program
		solana.NewAccountMeta(eventAuthority, false, false),
		solana.NewAccountMeta(programID, false, false),
	}

	return solana.NewInstruction(programID, accounts, data)
}
