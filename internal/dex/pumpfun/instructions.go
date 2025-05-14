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
	sellDiscriminator        = []byte{0x33, 0xe6, 0x85, 0xa4, 0x01, 0x7f, 0x83, 0xad}
	PumpFunExactSolProgramID = solana.MustPublicKeyFromBase58("6sbiyZ7mLKmYkES2AKYPHtg4FjQMaqVx3jTHez6ZtfmX")
	PumpFunProgramID         = solana.MustPublicKeyFromBase58("6EF8rrecthR5Dkzon8Nwu78hRvfCKubJ14M5uBEwF6P")
	PumpFunEventAuth         = solana.MustPublicKeyFromBase58("Ce6TQqeHC9p8KetsN6JsjHK7UTZk7nasjjnr7XxXp9F1")
	AssociatedTokenProgramID = solana.SPLAssociatedTokenAccountProgramID
	SystemProgramID          = solana.SystemProgramID
	TokenProgramID           = solana.TokenProgramID
	SysVarRentPubkey         = solana.SysVarRentPubkey
)

var extendDiscriminator = []byte{0xea, 0x66, 0xc2, 0xcb, 0x96, 0x48, 0x3e, 0xe5}

func createExtendAccountInstruction(
	accountPubkey,
	userPubkey,
	eventAuthority,
	programID solana.PublicKey,
) solana.Instruction {
	accounts := []*solana.AccountMeta{
		solana.NewAccountMeta(accountPubkey, true, false),
		solana.NewAccountMeta(userPubkey, false, true),
		solana.NewAccountMeta(SystemProgramID, false, false),
		solana.NewAccountMeta(eventAuthority, false, false),
		solana.NewAccountMeta(programID, false, false),
	}
	return solana.NewInstruction(programID, accounts, extendDiscriminator)
}

// createBuyExactSolInstruction создаёт инструкцию для покупки токена за точное
// количество SOL, включая новый creator_vault PDA.
func createBuyExactSolInstruction(
	global,
	feeRecipient,
	mint,
	bondingCurve,
	associatedBondingCurve,
	userATA,
	userWallet,
	creatorVault,
	eventAuthority,
	programID solana.PublicKey,
	solAmountLamports uint64,
) solana.Instruction {
	// 1) Кодируем количество SOL в 8 байт
	data := make([]byte, 8)
	binary.LittleEndian.PutUint64(data, solAmountLamports)

	// 2) Формируем список аккаунтов в соответствии с новой схемой:
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
		solana.NewAccountMeta(creatorVault, true, false), // ← новый параметр
		solana.NewAccountMeta(eventAuthority, false, false),
		solana.NewAccountMeta(programID, false, false),
	}

	// 3) Возвращаем инструкцию, указывая PID ExactSol
	return solana.NewInstruction(PumpFunExactSolProgramID, accounts, data)
}

// createSellInstruction создает инструкцию для продажи токенов в протоколе Pump.fun.
func createSellInstruction(
	programID,
	global,
	feeRecipient,
	mint,
	bondingCurve,
	associatedBC,
	userATA,
	userWallet,
	creatorVault,
	eventAuthority solana.PublicKey,
	amount,
	minSolOutput uint64,
) solana.Instruction {
	data := make([]byte, 24)
	copy(data[0:8], sellDiscriminator)
	binary.LittleEndian.PutUint64(data[8:16], amount)
	binary.LittleEndian.PutUint64(data[16:24], minSolOutput)

	accounts := []*solana.AccountMeta{
		{PublicKey: global, IsWritable: false, IsSigner: false},
		{PublicKey: feeRecipient, IsWritable: true, IsSigner: false},
		{PublicKey: mint, IsWritable: false, IsSigner: false},
		{PublicKey: bondingCurve, IsWritable: true, IsSigner: false},
		{PublicKey: associatedBC, IsWritable: true, IsSigner: false},
		{PublicKey: userATA, IsWritable: true, IsSigner: false},
		{PublicKey: userWallet, IsWritable: true, IsSigner: true},
		{PublicKey: SystemProgramID, IsWritable: false, IsSigner: false},
		{PublicKey: creatorVault, IsWritable: true, IsSigner: false}, // ← сюда
		{PublicKey: TokenProgramID, IsWritable: false, IsSigner: false},
		{PublicKey: eventAuthority, IsWritable: false, IsSigner: false},
		{PublicKey: programID, IsWritable: false, IsSigner: false},
	}
	return solana.NewInstruction(programID, accounts, data)
}
