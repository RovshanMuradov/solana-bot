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
// with optimized instruction construction and validation
func BuildBuyTokenInstruction(
	accounts InstructionAccounts,
	userWallet *wallet.Wallet,
	amount, maxSolCost uint64,
) (solana.Instruction, error) {
	// Use the exact discriminator from the protocol
	// This is the critical value "66063d1201daebea" as specified in the technical requirements
	buyDiscriminator := []byte{0x66, 0x06, 0x3d, 0x12, 0x01, 0xda, 0xeb, 0xea}

	// Validate account constraints before building instruction
	if accounts.Global.IsZero() {
		return nil, fmt.Errorf("global account address is required")
	}
	if accounts.FeeRecipient.IsZero() {
		return nil, fmt.Errorf("fee recipient address is required")
	}
	if accounts.Mint.IsZero() {
		return nil, fmt.Errorf("token mint address is required")
	}
	if accounts.BondingCurve.IsZero() {
		return nil, fmt.Errorf("bonding curve address is required")
	}
	if accounts.AssociatedBondingCurve.IsZero() {
		return nil, fmt.Errorf("associated bonding curve address is required")
	}
	if accounts.EventAuthority.IsZero() {
		return nil, fmt.Errorf("event authority address is required")
	}
	if accounts.Program.IsZero() {
		return nil, fmt.Errorf("program address is required")
	}

	// Create instruction data with precise byte layout:
	// 1. 8-byte discriminator prefix
	// 2. 8-byte little-endian encoded amount
	// 3. 8-byte little-endian encoded maxSolCost
	data := make([]byte, 24) // Pre-allocate for all data (8+8+8 bytes)
	
	// Copy discriminator (8 bytes)
	copy(data[0:8], buyDiscriminator)
	
	// Add amount in little-endian bytes (8 bytes)
	binary.LittleEndian.PutUint64(data[8:16], amount)
	
	// Add max SOL cost in little-endian bytes (8 bytes)
	binary.LittleEndian.PutUint64(data[16:24], maxSolCost)

	// Get user's associated token account using direct derivation
	associatedUser, _, err := solana.FindAssociatedTokenAddress(userWallet.PublicKey, accounts.Mint)
	if err != nil {
		return nil, fmt.Errorf("failed to derive associated token account: %w", err)
	}

	// Account list MUST follow exact protocol-mandated order as specified in tech requirements:
	// 1. global (non-writable, non-signer)
	// 2. feeRecipient (writable, non-signer)
	// 3. mint (non-writable, non-signer)
	// 4. bondingCurve (writable, non-signer)
	// 5. associatedBondingCurve (writable, non-signer)
	// 6. associatedUser (writable, non-signer)
	// 7. userPublicKey (writable, signer)
	// 8. systemProgram (non-writable, non-signer)
	// 9. tokenProgram (non-writable, non-signer)
	// 10. rent (non-writable, non-signer)
	// 11. eventAuthority (non-writable, non-signer)
	// 12. programID (non-writable, non-signer)
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
// with optimized instruction construction and validation
func BuildSellTokenInstruction(
	accounts InstructionAccounts,
	userWallet *wallet.Wallet,
	amount, minSolOutput uint64,
) (solana.Instruction, error) {
	// Use the exact discriminator from the protocol
	// This is the critical value "33e685a4017f83ad" as specified in the technical requirements
	sellDiscriminator := []byte{0x33, 0xe6, 0x85, 0xa4, 0x01, 0x7f, 0x83, 0xad}

	// Validate account constraints before building instruction
	if accounts.Global.IsZero() {
		return nil, fmt.Errorf("global account address is required")
	}
	if accounts.FeeRecipient.IsZero() {
		return nil, fmt.Errorf("fee recipient address is required")
	}
	if accounts.Mint.IsZero() {
		return nil, fmt.Errorf("token mint address is required")
	}
	if accounts.BondingCurve.IsZero() {
		return nil, fmt.Errorf("bonding curve address is required")
	}
	if accounts.AssociatedBondingCurve.IsZero() {
		return nil, fmt.Errorf("associated bonding curve address is required")
	}
	if accounts.EventAuthority.IsZero() {
		return nil, fmt.Errorf("event authority address is required")
	}
	if accounts.Program.IsZero() {
		return nil, fmt.Errorf("program address is required")
	}

	// Create instruction data with precise byte layout:
	// 1. 8-byte discriminator prefix
	// 2. 8-byte little-endian encoded amount
	// 3. 8-byte little-endian encoded minSolOutput
	data := make([]byte, 24) // Pre-allocate for all data (8+8+8 bytes)
	
	// Copy discriminator (8 bytes)
	copy(data[0:8], sellDiscriminator)
	
	// Add amount in little-endian bytes (8 bytes)
	binary.LittleEndian.PutUint64(data[8:16], amount)
	
	// Add min SOL output in little-endian bytes (8 bytes)
	binary.LittleEndian.PutUint64(data[16:24], minSolOutput)

	// Get user's associated token account using direct derivation
	associatedUser, _, err := solana.FindAssociatedTokenAddress(userWallet.PublicKey, accounts.Mint)
	if err != nil {
		return nil, fmt.Errorf("failed to derive associated token account: %w", err)
	}

	// Account list must be in the exact order expected by the program
	// The sell instruction needs all the same accounts as buy, plus the Associated Token Program
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

// CreateDiscriminator is the instruction discriminator for creating a token and its bonding curve
var CreateDiscriminator = []byte{0x20, 0xca, 0xb0, 0x52, 0xf7, 0x6d, 0xd0, 0x57}

// CreateComputeBudgetInstructions creates the compute budget instructions for Solana transactions
// Returns two instructions: one to set compute unit limit and one to set compute unit price
func CreateComputeBudgetInstructions(computeUnits uint32, microLamportsPerUnit uint64) ([]solana.Instruction, error) {
	// Compute Budget Program ID
	computeBudgetProgramID := solana.MustPublicKeyFromBase58("ComputeBudget111111111111111111111111111111")
	
	// Create instruction array
	instructions := make([]solana.Instruction, 2)
	
	// Create set compute unit limit instruction
	// Instruction data: first byte is 0x02 (instruction index), followed by 4-byte little-endian uint32
	unitLimitData := []byte{0x02, 0x00, 0x00, 0x00, 0x00}
	binary.LittleEndian.PutUint32(unitLimitData[1:], computeUnits)
	
	instructions[0] = solana.NewInstruction(
		computeBudgetProgramID,
		[]*solana.AccountMeta{}, // No accounts needed
		unitLimitData,
	)
	
	// Create set compute unit price instruction
	// Instruction data: first byte is 0x03 (instruction index), followed by 8-byte little-endian uint64
	unitPriceData := []byte{0x03, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	binary.LittleEndian.PutUint64(unitPriceData[1:], microLamportsPerUnit)
	
	instructions[1] = solana.NewInstruction(
		computeBudgetProgramID,
		[]*solana.AccountMeta{}, // No accounts needed
		unitPriceData,
	)
	
	return instructions, nil
}

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
