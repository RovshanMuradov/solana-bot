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
)

// PumpFunInstructionParams содержит все параметры для создания инструкций Pump.fun
type PumpFunInstructionParams struct {
	// Общие параметры
	Global                 solana.PublicKey
	FeeRecipient           solana.PublicKey
	Mint                   solana.PublicKey
	BondingCurve           solana.PublicKey
	AssociatedBondingCurve solana.PublicKey
	UserATA                solana.PublicKey
	UserWallet             solana.PublicKey
	EventAuthority         solana.PublicKey
	ProgramID              solana.PublicKey

	// Параметры операций
	SolAmountLamports uint64 // Для buy exact SOL
	TokenAmount       uint64 // Для sell
	MinSolOutput      uint64 // Для sell
}

// createBuyExactSolInstruction creates an instruction for buying with an exact SOL amount
func createBuyExactSolInstruction(params *PumpFunInstructionParams) solana.Instruction {
	// Create instruction data - only 8 bytes for sol amount in lamports
	data := make([]byte, 8)
	binary.LittleEndian.PutUint64(data, params.SolAmountLamports)

	// Account list follows the same order as regular buy instruction
	accounts := []*solana.AccountMeta{
		solana.NewAccountMeta(params.Global, false, false),
		solana.NewAccountMeta(params.FeeRecipient, false, true),
		solana.NewAccountMeta(params.Mint, false, false),
		solana.NewAccountMeta(params.BondingCurve, false, true),
		solana.NewAccountMeta(params.AssociatedBondingCurve, false, true),
		solana.NewAccountMeta(params.UserATA, false, true),
		solana.NewAccountMeta(params.UserWallet, true, true),
		solana.NewAccountMeta(solana.SystemProgramID, false, false),
		solana.NewAccountMeta(solana.TokenProgramID, false, false),
		solana.NewAccountMeta(solana.SysVarRentPubkey, false, false),
		solana.NewAccountMeta(params.EventAuthority, false, false),
		solana.NewAccountMeta(PumpFunProgramID, false, false),
	}

	return solana.NewInstruction(PumpFunExactSolProgramID, accounts, data)
}

// createSellInstruction creates a sell instruction for the Pump.fun protocol
func createSellInstruction(params *PumpFunInstructionParams) solana.Instruction {
	// Create instruction data with precise byte layout:
	// 1. 8-byte discriminator prefix
	// 2. 8-byte little-endian encoded amount
	// 3. 8-byte little-endian encoded minSolOutput
	data := make([]byte, 24)

	// Copy discriminator (8 bytes)
	copy(data[0:8], sellDiscriminator)

	// Add amount in little-endian bytes (8 bytes)
	binary.LittleEndian.PutUint64(data[8:16], params.TokenAmount)

	// Add min SOL output in little-endian bytes (8 bytes)
	binary.LittleEndian.PutUint64(data[16:24], params.MinSolOutput)

	// Account list MUST follow exact protocol-mandated order
	accounts := []*solana.AccountMeta{
		solana.NewAccountMeta(params.Global, false, false),
		solana.NewAccountMeta(params.FeeRecipient, false, true),
		solana.NewAccountMeta(params.Mint, false, false),
		solana.NewAccountMeta(params.BondingCurve, false, true),
		solana.NewAccountMeta(params.AssociatedBondingCurve, false, true),
		solana.NewAccountMeta(params.UserATA, false, true),
		solana.NewAccountMeta(params.UserWallet, true, true),
		solana.NewAccountMeta(solana.SystemProgramID, false, false),
		solana.NewAccountMeta(AssociatedTokenProgramID, false, false),
		solana.NewAccountMeta(solana.TokenProgramID, false, false),
		solana.NewAccountMeta(params.EventAuthority, false, false),
		solana.NewAccountMeta(params.ProgramID, false, false),
	}

	return solana.NewInstruction(params.ProgramID, accounts, data)
}
