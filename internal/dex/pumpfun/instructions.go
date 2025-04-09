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

// createBuyExactSolInstruction создает инструкцию для покупки токена с фиксированным количеством SOL.
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
	// Создаем данные инструкции - 8 байт для количества SOL в ламппортах
	// ExactSol программа требует только параметр количества SOL
	data := make([]byte, 8)
	binary.LittleEndian.PutUint64(data, solAmountLamports)

	// Формируем список аккаунтов, участвующих в инструкции.
	// Порядок аккаунтов строго соответствует ожидаемому смарт-контрактом.
	// Параметры для каждого аккаунта: адрес, можно ли изменять (writable), является ли подписывающим (signer)
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

	// Возвращаем готовую инструкцию, указывая программу ExactSol, список аккаунтов и данные
	return solana.NewInstruction(PumpFunExactSolProgramID, accounts, data)
}

// createSellInstruction создает инструкцию для продажи токенов в протоколе Pump.fun.
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
	// Создаем данные инструкции:
	// - 8 байт дискриминатор (идентификатор функции sell)
	// - 8 байт количество токенов для продажи
	// - 8 байт минимальный выход SOL для защиты от проскальзывания
	data := make([]byte, 24)

	// Копируем дискриминатор в начало данных
	copy(data[0:8], sellDiscriminator)

	// Записываем количество токенов в формате little-endian
	binary.LittleEndian.PutUint64(data[8:16], amount)

	// Записываем минимальный выход SOL в формате little-endian
	binary.LittleEndian.PutUint64(data[16:24], minSolOutput)

	// Формируем список аккаунтов, необходимых для выполнения инструкции
	// Порядок параметров в NewAccountMeta: pubKey, IsWritable, IsSigner
	accounts := []*solana.AccountMeta{
		solana.NewAccountMeta(global, false, false),
		solana.NewAccountMeta(feeRecipient, true, false),
		solana.NewAccountMeta(mint, false, false),
		solana.NewAccountMeta(bondingCurve, true, false),
		solana.NewAccountMeta(associatedBondingCurve, true, false),
		solana.NewAccountMeta(userATA, true, false),
		solana.NewAccountMeta(userWallet, true, true),
		solana.NewAccountMeta(SystemProgramID, false, false),
		solana.NewAccountMeta(AssociatedTokenProgramID, false, false),
		solana.NewAccountMeta(TokenProgramID, false, false),
		solana.NewAccountMeta(eventAuthority, false, false),
		solana.NewAccountMeta(programID, false, false),
	}

	// Возвращаем готовую инструкцию с указанной программой, списком аккаунтов и данными
	return solana.NewInstruction(programID, accounts, data)
}
