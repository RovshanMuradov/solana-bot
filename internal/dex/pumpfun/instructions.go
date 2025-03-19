// ==============================================
// File: internal/dex/pumpfun/instructions.go
// ==============================================
package pumpfun

import (
	"context"
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/gagliardetto/solana-go"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc"
	"github.com/rovshanmuradov/solana-bot/internal/wallet"
	"go.uber.org/zap"
)

// BuildBuyTokenInstruction builds a buy instruction for Pump.fun protocol
func BuildBuyTokenInstruction(
	accounts InstructionAccounts,
	userWallet *wallet.Wallet,
	amount, maxSolCost uint64,
) (solana.Instruction, error) {
	// Use configured discriminator version
	discriminator, ok := BuyDiscriminators[DiscriminatorVersion]
	if !ok {
		return nil, fmt.Errorf("discriminator version %s not found", DiscriminatorVersion)
	}

	// Create instruction data
	data := make([]byte, len(discriminator))
	copy(data, discriminator)

	// Add amount in little-endian bytes
	amountBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(amountBytes, amount)
	data = append(data, amountBytes...)

	// Add max SOL cost in little-endian bytes
	maxSolBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(maxSolBytes, maxSolCost)
	data = append(data, maxSolBytes...)

	// Get user's associated token account
	associatedUser, err := userWallet.GetATA(accounts.Mint)
	if err != nil {
		return nil, fmt.Errorf("failed to get associated token account: %w", err)
	}

	// Account list must be in the exact order expected by the program
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

	// Create and return the instruction
	return solana.NewInstruction(accounts.Program, insAccounts, data), nil
}

// BuildSellTokenInstruction builds a sell instruction for Pump.fun protocol
func BuildSellTokenInstruction(
	accounts InstructionAccounts,
	userWallet *wallet.Wallet,
	amount, minSolOutput uint64,
) (solana.Instruction, error) {
	// Use configured discriminator version
	discriminator, ok := SellDiscriminators[DiscriminatorVersion]
	if !ok {
		return nil, fmt.Errorf("discriminator version %s not found", DiscriminatorVersion)
	}

	// Create instruction data
	data := make([]byte, len(discriminator))
	copy(data, discriminator)

	// Add amount in little-endian bytes
	amountBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(amountBytes, amount)
	data = append(data, amountBytes...)

	// Add min SOL output in little-endian bytes
	minSolBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(minSolBytes, minSolOutput)
	data = append(data, minSolBytes...)

	// Get user's associated token account
	associatedUser, err := userWallet.GetATA(accounts.Mint)
	if err != nil {
		return nil, fmt.Errorf("failed to get associated token account: %w", err)
	}

	// Account list must be in the exact order expected by the program
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
		{PublicKey: AssociatedTokenProgramID, IsSigner: false, IsWritable: false},
		{PublicKey: accounts.EventAuthority, IsSigner: false, IsWritable: false},
		{PublicKey: accounts.Program, IsSigner: false, IsWritable: false},
	}

	// Create and return the instruction
	return solana.NewInstruction(accounts.Program, insAccounts, data), nil
}

// InitializeAssociatedBondingCurveDiscriminator is the instruction discriminator for initializing 
// an associated bonding curve account
var InitializeAssociatedBondingCurveDiscriminator = []byte{0x12, 0x65, 0x4a, 0xb9, 0x32, 0x67, 0xcd, 0xaa}

// BuildInitializeAssociatedBondingCurveInstruction builds an instruction to initialize
// the associated bonding curve account with the Pump.fun program
func BuildInitializeAssociatedBondingCurveInstruction(
	mint solana.PublicKey,
	bondingCurve solana.PublicKey,
	associatedBondingCurve solana.PublicKey,
	programID solana.PublicKey,
	payer solana.PublicKey,
) solana.Instruction {
	// Create instruction data with the initialize discriminator
	data := make([]byte, len(InitializeAssociatedBondingCurveDiscriminator))
	copy(data, InitializeAssociatedBondingCurveDiscriminator)

	// Get bonding curve ATA
	bondingCurveATA, _, _ := solana.FindAssociatedTokenAddress(bondingCurve, mint)

	// Account list in the exact order expected by the program for initialization
	insAccounts := []*solana.AccountMeta{
		{PublicKey: bondingCurve, IsSigner: false, IsWritable: true},
		{PublicKey: associatedBondingCurve, IsSigner: false, IsWritable: true},
		{PublicKey: bondingCurveATA, IsSigner: false, IsWritable: false},
		{PublicKey: mint, IsSigner: false, IsWritable: false},
		{PublicKey: payer, IsSigner: true, IsWritable: true},
		{PublicKey: solana.SystemProgramID, IsSigner: false, IsWritable: false},
		{PublicKey: solana.TokenProgramID, IsSigner: false, IsWritable: false},
		{PublicKey: SysvarRentPubkey, IsSigner: false, IsWritable: false},
	}

	// Create and return the instruction
	return solana.NewInstruction(programID, insAccounts, data)
}

// createAssociatedTokenAccount creates the associated token account if it doesn't exist
func createAssociatedTokenAccount(
	ctx context.Context,
	client *solbc.Client,
	payer *wallet.Wallet,
	mint solana.PublicKey,
	owner solana.PublicKey,
	_ *zap.Logger,
) (*solana.Transaction, error) {
	// Get the associated token address
	associatedAddress, _, err := solana.FindAssociatedTokenAddress(owner, mint)
	if err != nil {
		return nil, fmt.Errorf("failed to get associated token address: %w", err)
	}

	// Check if the account already exists
	exists, err := accountExists(ctx, client, associatedAddress)
	if err != nil {
		return nil, err
	}

	// If account already exists, return nil (no need to create)
	if exists {
		return nil, nil
	}

	// Get recent blockhash
	blockhash, err := client.GetRecentBlockhash(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get recent blockhash: %w", err)
	}

	// Create instruction to create associated token account
	createIx := createAssociatedTokenAccountInstruction(payer.PublicKey, associatedAddress, owner, mint)

	// Create transaction
	tx, err := solana.NewTransaction(
		[]solana.Instruction{createIx},
		blockhash,
		solana.TransactionPayer(payer.PublicKey),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create transaction: %w", err)
	}

	return tx, nil
}

func accountExists(ctx context.Context, client *solbc.Client, address solana.PublicKey) (bool, error) {
	accountInfo, err := client.GetAccountInfo(ctx, address)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return false, nil // Аккаунт не существует, это нормально
		}
		return false, fmt.Errorf("failed to check account existence: %w", err)
	}
	return accountInfo != nil && accountInfo.Value != nil, nil
}

func createAssociatedTokenAccountInstruction(payer, associatedAddress, owner, mint solana.PublicKey) solana.Instruction {
	keys := []*solana.AccountMeta{
		{PublicKey: payer, IsSigner: true, IsWritable: true},
		{PublicKey: associatedAddress, IsSigner: false, IsWritable: true},
		{PublicKey: owner, IsSigner: false, IsWritable: false},
		{PublicKey: mint, IsSigner: false, IsWritable: false},
		{PublicKey: solana.SystemProgramID, IsSigner: false, IsWritable: false},
		{PublicKey: solana.TokenProgramID, IsSigner: false, IsWritable: false},
		{PublicKey: AssociatedTokenProgramID, IsSigner: false, IsWritable: false},
	}

	return solana.NewInstruction(
		AssociatedTokenProgramID,
		keys,
		[]byte{},
	)
}
