// ==============================================
// File: internal/dex/pumpfun/instructions.go
// ==============================================
package pumpfun

import (
	"context"
	"encoding/binary"
	"fmt"
	"strings"
	"time"

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

// CreateDiscriminator is the instruction discriminator for creating a token and its bonding curve
var CreateDiscriminator = []byte{0x20, 0xca, 0xb0, 0x52, 0xf7, 0x6d, 0xd0, 0x57}

// VerifyBondingCurveInstruction builds an instruction to verify the existence and ownership
// of the bonding curve and associated bonding curve accounts
func VerifyBondingCurveInstruction(
	ctx context.Context,
	client *solbc.Client,
	mint solana.PublicKey,
	bondingCurve solana.PublicKey,
	associatedBondingCurve solana.PublicKey,
	wallet *wallet.Wallet,
	logger *zap.Logger,
) (bool, error) {
	// Create a timeout context for verification operations
	verifyCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Check if bonding curve exists and is owned by the program
	bcInfo, err := client.GetAccountInfo(verifyCtx, bondingCurve)
	if err != nil {
		// Handle "not found" error specifically for better diagnostics
		if strings.Contains(err.Error(), "not found") {
			logger.Warn("Bonding curve account does not exist",
				zap.String("bonding_curve", bondingCurve.String()))
			return false, nil
		}
		return false, fmt.Errorf("failed to get bonding curve info: %w", err)
	}

	// Check if bonding curve account exists and is properly initialized
	if bcInfo.Value == nil {
		logger.Warn("Bonding curve account response is empty",
			zap.String("bonding_curve", bondingCurve.String()))
		return false, nil
	}

	// Check ownership of bonding curve account
	validBondingCurve := bcInfo.Value.Owner.Equals(PumpFunProgramID)
	if !validBondingCurve {
		logger.Warn("Bonding curve exists but has incorrect ownership",
			zap.String("bonding_curve", bondingCurve.String()),
			zap.String("owner", bcInfo.Value.Owner.String()),
			zap.String("expected_owner", PumpFunProgramID.String()))
		return false, nil
	}

	// Check if associated bonding curve exists and is owned by the program
	abcInfo, err := client.GetAccountInfo(verifyCtx, associatedBondingCurve)
	if err != nil {
		// Handle "not found" error specifically - the critical fix
		if strings.Contains(err.Error(), "not found") {
			logger.Error("Associated bonding curve account does not exist",
				zap.String("associated_bonding_curve", associatedBondingCurve.String()))
			
			// CRITICAL: According to the Pump.fun protocol specifications,
			// the associated bonding curve MUST be initialized before transactions.
			// The account is NOT automatically created by the protocol during transactions.
			logger.Info("Associated bonding curve must be properly initialized before transactions", 
				zap.String("mint", mint.String()),
				zap.String("bonding_curve", bondingCurve.String()))
				
			return false, nil  // Return failure to prevent proceeding with uninitialized account
		}
		return false, fmt.Errorf("failed to get associated bonding curve info: %w", err)
	}

	// Check if associated bonding curve account exists
	if abcInfo.Value == nil {
		logger.Error("Associated bonding curve account response is empty",
			zap.String("associated_bonding_curve", associatedBondingCurve.String()))
		
		// Don't allow operation to proceed with empty account data
		logger.Info("Cannot proceed with empty associated bonding curve account", 
			zap.String("mint", mint.String()),
			zap.String("bonding_curve", bondingCurve.String()))
			
		return false, nil
	}

	// Check ownership of associated bonding curve if it exists
	validAssociatedBondingCurve := abcInfo.Value.Owner.Equals(PumpFunProgramID)
	if !validAssociatedBondingCurve {
		logger.Error("Associated bonding curve exists but has incorrect ownership",
			zap.String("associated_bonding_curve", associatedBondingCurve.String()),
			zap.String("owner", abcInfo.Value.Owner.String()),
			zap.String("expected_owner", PumpFunProgramID.String()))
		
		// Don't allow operation to proceed with incorrect ownership
		logger.Info("Cannot proceed with incorrectly owned associated bonding curve", 
			zap.String("mint", mint.String()),
			zap.String("bonding_curve", bondingCurve.String()))
			
		return false, nil
	}

	logger.Debug("Bonding curve accounts verified successfully")
	return true, nil
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
