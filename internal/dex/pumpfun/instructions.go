// ==============================================
// File: internal/dex/pumpfun/instructions.go
// ==============================================
package pumpfun

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/rovshanmuradov/solana-bot/internal/wallet"
	"go.uber.org/zap"
)

// Constants
var (
	SysvarRentPubkey         = solana.MustPublicKeyFromBase58("SysvarRent111111111111111111111111111111111")
	AssociatedTokenProgramID = solana.MustPublicKeyFromBase58("ATokenGPvbdGVxr1b2hvZbsiqW5xWH25efTNsLJA8knL")
	EventAuthorityAddress    = solana.MustPublicKeyFromBase58("Ce6TQqeHC9p8KetsN6JsjHK7UTZk7nasjjnr7XxXp9F1")

	// Version control for instruction discriminators
	DiscriminatorVersion = "v3" // Control which version to use
)

// Discriminator versions to try - add more as needed
var (
	// Buy discriminators directly from the Pump.fun SDK IDL
	BuyDiscriminators = map[string][]byte{
		"v1": {0x66, 0x06, 0x3d, 0x12},                         // Partial (deprecated)
		"v2": {0xd4, 0x52, 0x39, 0xd5, 0xf6, 0x27, 0x64, 0x9b}, // Wrong attempt
		"v3": {0x66, 0x06, 0x3d, 0x12, 0x01, 0xda, 0xeb, 0xea}, // Correct full discriminator from SDK
	}

	// Sell discriminators directly from the Pump.fun SDK IDL
	SellDiscriminators = map[string][]byte{
		"v1": {0x33, 0xe6, 0x85, 0xa4},                         // Partial (deprecated)
		"v2": {0x28, 0x17, 0x38, 0x89, 0x55, 0x34, 0xde, 0xd5}, // Wrong attempt
		"v3": {0x33, 0xe6, 0x85, 0xa4, 0x01, 0x7f, 0x83, 0xad}, // Correct full discriminator from SDK
	}
)

// calculateMethodDiscriminator generates Anchor-compatible method discriminator
// This follows Anchor's convention: first 8 bytes of sha256("method_name")
// Note: Kept for debugging purposes, but we're using hardcoded values from SDK
func calculateMethodDiscriminator(methodName string) []byte {
	hash := sha256.Sum256([]byte(methodName))
	return hash[:8]
}

// BuyInstructionAccounts holds account references for buy operation
type BuyInstructionAccounts struct {
	Global                 solana.PublicKey
	FeeRecipient           solana.PublicKey
	Mint                   solana.PublicKey
	BondingCurve           solana.PublicKey
	AssociatedBondingCurve solana.PublicKey
	EventAuthority         solana.PublicKey
	Program                solana.PublicKey
	Logger                 *zap.Logger // Add logger for debugging
}

// SellInstructionAccounts holds account references for sell operation
type SellInstructionAccounts struct {
	Global                 solana.PublicKey
	FeeRecipient           solana.PublicKey
	Mint                   solana.PublicKey
	BondingCurve           solana.PublicKey
	AssociatedBondingCurve solana.PublicKey
	EventAuthority         solana.PublicKey
	Program                solana.PublicKey
	Logger                 *zap.Logger // Add logger for debugging
}

// BuildBuyTokenInstruction builds a buy instruction for Pump.fun protocol
func BuildBuyTokenInstruction(
	accounts BuyInstructionAccounts,
	userWallet *wallet.Wallet,
	amount, maxSolCost uint64,
) (solana.Instruction, error) {
	// Use configured discriminator version
	discriminator, ok := BuyDiscriminators[DiscriminatorVersion]
	if !ok {
		return nil, fmt.Errorf("discriminator version %s not found", DiscriminatorVersion)
	}

	// Log discriminator information
	if accounts.Logger != nil {
		accounts.Logger.Debug("Using buy instruction discriminator",
			zap.String("version", DiscriminatorVersion),
			zap.String("hex", hex.EncodeToString(discriminator)),
			zap.Int("length", len(discriminator)))
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

	// Log account addresses for debugging
	if accounts.Logger != nil {
		accounts.Logger.Debug("Building buy instruction with accounts",
			zap.String("global", accounts.Global.String()),
			zap.String("feeRecipient", accounts.FeeRecipient.String()),
			zap.String("mint", accounts.Mint.String()),
			zap.String("bondingCurve", accounts.BondingCurve.String()),
			zap.String("associatedBondingCurve", accounts.AssociatedBondingCurve.String()),
			zap.String("userATA", associatedUser.String()),
			zap.String("userWallet", userWallet.PublicKey.String()))
	}

	// Account list must be in the exact order expected by the program
	// Matching accounts from SDK: "buy" instruction accounts order
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

	// Log full instruction data
	if accounts.Logger != nil {
		accounts.Logger.Debug("Buy instruction data",
			zap.String("hex", hex.EncodeToString(data)),
			zap.Int("dataLength", len(data)),
			zap.Uint64("amount", amount),
			zap.Uint64("maxSolCost", maxSolCost))
	}

	// Create and return the instruction
	return solana.NewInstruction(accounts.Program, insAccounts, data), nil
}

// BuildSellTokenInstruction builds a sell instruction for Pump.fun protocol
func BuildSellTokenInstruction(
	accounts SellInstructionAccounts,
	userWallet *wallet.Wallet,
	amount, minSolOutput uint64,
) (solana.Instruction, error) {
	// Use configured discriminator version
	discriminator, ok := SellDiscriminators[DiscriminatorVersion]
	if !ok {
		return nil, fmt.Errorf("discriminator version %s not found", DiscriminatorVersion)
	}

	// Log discriminator information
	if accounts.Logger != nil {
		accounts.Logger.Debug("Using sell instruction discriminator",
			zap.String("version", DiscriminatorVersion),
			zap.String("hex", hex.EncodeToString(discriminator)),
			zap.Int("length", len(discriminator)))
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

	// Log account addresses for debugging
	if accounts.Logger != nil {
		accounts.Logger.Debug("Building sell instruction with accounts",
			zap.String("global", accounts.Global.String()),
			zap.String("feeRecipient", accounts.FeeRecipient.String()),
			zap.String("mint", accounts.Mint.String()),
			zap.String("bondingCurve", accounts.BondingCurve.String()),
			zap.String("associatedBondingCurve", accounts.AssociatedBondingCurve.String()),
			zap.String("userATA", associatedUser.String()),
			zap.String("userWallet", userWallet.PublicKey.String()))
	}

	// Account list must be in the exact order expected by the program
	// Matching accounts from SDK: "sell" instruction accounts order
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

	// Log full instruction data
	if accounts.Logger != nil {
		accounts.Logger.Debug("Sell instruction data",
			zap.String("hex", hex.EncodeToString(data)),
			zap.Int("dataLength", len(data)),
			zap.Uint64("amount", amount),
			zap.Uint64("minSolOutput", minSolOutput))
	}

	// Create and return the instruction
	return solana.NewInstruction(accounts.Program, insAccounts, data), nil
}

// TryBuildBuyTokenInstructions builds buy instructions with all available discriminator versions
// This helps to debug which discriminator version works with the program
func TryBuildBuyTokenInstructions(
	accounts BuyInstructionAccounts,
	userWallet *wallet.Wallet,
	amount, maxSolCost uint64,
) ([]solana.Instruction, error) {
	var instructions []solana.Instruction

	// Try all available discriminator versions
	for version := range BuyDiscriminators {
		originalVersion := DiscriminatorVersion
		DiscriminatorVersion = version

		instruction, err := BuildBuyTokenInstruction(accounts, userWallet, amount, maxSolCost)
		if err != nil {
			// Restore original version and return error
			DiscriminatorVersion = originalVersion
			return nil, fmt.Errorf("failed to build buy instruction with version %s: %w", version, err)
		}

		instructions = append(instructions, instruction)
	}

	// Restore original version
	DiscriminatorVersion = "v3"

	return instructions, nil
}

// ValidateDiscriminators verifies if the hardcoded discriminators match expected Anchor calculations
// This is useful for debugging only
func ValidateDiscriminators() bool {
	// Calculate expected discriminators
	expectedBuy := calculateMethodDiscriminator("buy")
	expectedSell := calculateMethodDiscriminator("sell")

	// Compare with hardcoded values
	buyMatches := hex.EncodeToString(BuyDiscriminators["v3"]) == hex.EncodeToString(expectedBuy)
	sellMatches := hex.EncodeToString(SellDiscriminators["v3"]) == hex.EncodeToString(expectedSell)

	if !buyMatches {
		fmt.Printf("Buy discriminator mismatch: hardcoded=%s, calculated=%s\n",
			hex.EncodeToString(BuyDiscriminators["v3"]),
			hex.EncodeToString(expectedBuy))
	}

	if !sellMatches {
		fmt.Printf("Sell discriminator mismatch: hardcoded=%s, calculated=%s\n",
			hex.EncodeToString(SellDiscriminators["v3"]),
			hex.EncodeToString(expectedSell))
	}

	return buyMatches && sellMatches
}
