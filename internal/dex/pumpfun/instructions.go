// internal/dex/pumpfun/instructions.go
package pumpfun

import (
	"encoding/binary"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/rovshanmuradov/solana-bot/internal/wallet"
)

// Определяем SysvarRentPubkey и AssociatedTokenProgramID.
var SysvarRentPubkey = solana.MustPublicKeyFromBase58("SysvarRent111111111111111111111111111111111")
var AssociatedTokenProgramID = solana.MustPublicKeyFromBase58("ATokenGPvbdGVxr1b2hvZbsiqW5xWH25efTNsLJA8knL")

// Структуры для передачи аккаунтов.
type BuyInstructionAccounts struct {
	Global                 solana.PublicKey // Глобальный аккаунт программы.
	FeeRecipient           solana.PublicKey // Аккаунт для комиссий.
	Mint                   solana.PublicKey // Аккаунт mint токена.
	BondingCurve           solana.PublicKey // Bonding curve аккаунт.
	AssociatedBondingCurve solana.PublicKey // Ассоциированный bonding curve аккаунт.
	EventAuthority         solana.PublicKey // Аккаунт событий.
	Program                solana.PublicKey // Адрес программы Pump.fun.
}

type SellInstructionAccounts struct {
	Global                 solana.PublicKey // Глобальный аккаунт.
	FeeRecipient           solana.PublicKey // Аккаунт комиссий.
	Mint                   solana.PublicKey // Mint токена.
	BondingCurve           solana.PublicKey // Bonding curve аккаунт.
	AssociatedBondingCurve solana.PublicKey // Ассоциированный bonding curve.
	EventAuthority         solana.PublicKey // Аккаунт событий.
	Program                solana.PublicKey // Адрес программы Pump.fun.
}

// BuildBuyTokenInstruction формирует инструкцию покупки токена.
func BuildBuyTokenInstruction(
	accounts BuyInstructionAccounts,
	userWallet *wallet.Wallet,
	amount, maxSolCost uint64,
) (solana.Instruction, error) {
	// Сериализация данных инструкции.
	data := []byte{0x66, 0x06, 0x3d, 0x12} // код инструкции Pump.fun для buy
	amountBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(amountBytes, amount)
	data = append(data, amountBytes...)
	maxSolBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(maxSolBytes, maxSolCost)
	data = append(data, maxSolBytes...)
	data = append(data, make([]byte, 4)...) // padding

	// Получаем associated token account пользователя через ATA.
	associatedUser, err := userWallet.GetATA(accounts.Mint)
	if err != nil {
		return nil, fmt.Errorf("failed to get associated token account: %w", err)
	}

	// Формируем список аккаунтов согласно спецификации.
	insAccounts := []*solana.AccountMeta{
		{PublicKey: accounts.Global, IsSigner: false, IsWritable: false},
		{PublicKey: accounts.FeeRecipient, IsSigner: false, IsWritable: true},
		{PublicKey: accounts.Mint, IsSigner: false, IsWritable: false},
		{PublicKey: accounts.BondingCurve, IsSigner: false, IsWritable: true},
		{PublicKey: accounts.AssociatedBondingCurve, IsSigner: false, IsWritable: true},
		{PublicKey: associatedUser, IsSigner: false, IsWritable: true},
		{PublicKey: userWallet.PublicKey, IsSigner: true, IsWritable: true},
		{PublicKey: solana.SystemProgramID, IsSigner: false, IsWritable: false},
		{PublicKey: solana.TokenProgramID, IsSigner: false, IsWritable: false},
		{PublicKey: SysvarRentPubkey, IsSigner: false, IsWritable: false},
		{PublicKey: accounts.EventAuthority, IsSigner: false, IsWritable: false},
		{PublicKey: accounts.Program, IsSigner: false, IsWritable: false},
	}

	return solana.NewInstruction(accounts.Program, insAccounts, data), nil
}

// BuildSellTokenInstruction формирует инструкцию продажи токена.
func BuildSellTokenInstruction(
	accounts SellInstructionAccounts,
	userWallet *wallet.Wallet,
	amount, minSolOutput uint64,
) (solana.Instruction, error) {
	data := []byte{0x33, 0xe6, 0x85, 0xa4}
	amountBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(amountBytes, amount)
	data = append(data, amountBytes...)
	minSolBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(minSolBytes, minSolOutput)
	data = append(data, minSolBytes...)
	data = append(data, make([]byte, 4)...) // padding

	associatedUser, err := userWallet.GetATA(accounts.Mint)
	if err != nil {
		return nil, fmt.Errorf("failed to get associated token account: %w", err)
	}

	insAccounts := []*solana.AccountMeta{
		{PublicKey: accounts.Global, IsSigner: false, IsWritable: false},
		{PublicKey: accounts.FeeRecipient, IsSigner: false, IsWritable: true},
		{PublicKey: accounts.Mint, IsSigner: false, IsWritable: false},
		{PublicKey: accounts.BondingCurve, IsSigner: false, IsWritable: true},
		{PublicKey: accounts.AssociatedBondingCurve, IsSigner: false, IsWritable: true},
		{PublicKey: associatedUser, IsSigner: false, IsWritable: true},
		{PublicKey: userWallet.PublicKey, IsSigner: true, IsWritable: true},
		{PublicKey: solana.SystemProgramID, IsSigner: false, IsWritable: false},
		{PublicKey: AssociatedTokenProgramID, IsSigner: false, IsWritable: false},
		{PublicKey: solana.TokenProgramID, IsSigner: false, IsWritable: false},
		{PublicKey: accounts.EventAuthority, IsSigner: false, IsWritable: false},
		{PublicKey: accounts.Program, IsSigner: false, IsWritable: false},
	}

	return solana.NewInstruction(accounts.Program, insAccounts, data), nil
}
