// internal/dex/raydium/transaction.go

package raydium

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"

	"github.com/gagliardetto/solana-go"
	computebudget "github.com/gagliardetto/solana-go/programs/compute-budget"
	"github.com/rovshanmuradov/solana-bot/internal/types"
	"github.com/rovshanmuradov/solana-bot/internal/wallet"
	"go.uber.org/zap"
)

// Serialize serializes the swap instruction data
func (s *SwapInstructionData) Serialize() ([]byte, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}

	data := make([]byte, 17)

	// Write instruction discriminator
	data[0] = s.Instruction

	// Write amount in
	binary.LittleEndian.PutUint64(data[1:9], s.AmountIn)

	// Write minimum out
	binary.LittleEndian.PutUint64(data[9:17], s.MinAmountOut)

	return data, nil
}

// Validate validates the swap instruction data
func (s *SwapInstructionData) Validate() error {
	if s.Instruction != 1 {
		return fmt.Errorf("invalid instruction type: expected 1, got %d", s.Instruction)
	}

	if s.AmountIn == 0 {
		return fmt.Errorf("amount_in cannot be zero")
	}

	// MinimumOut can be zero, but log a warning
	if s.MinAmountOut == 0 {
		// You may want to log a warning here
	}

	return nil
}

// CreateSwapInstruction creates a swap instruction for Raydium
func (r *DEX) CreateSwapInstruction(
	userWallet solana.PublicKey,
	userSourceTokenAccount solana.PublicKey,
	userDestinationTokenAccount solana.PublicKey,
	amountIn uint64,
	minAmountOut uint64,
	logger *zap.Logger,
	poolInfo *Pool,
) (solana.Instruction, error) {
	logger.Debug("Creating swap instruction",
		zap.String("user_wallet", userWallet.String()),
		zap.String("source_account", userSourceTokenAccount.String()),
		zap.String("destination_account", userDestinationTokenAccount.String()),
		zap.Uint64("amount_in", amountIn),
		zap.Uint64("min_amount_out", minAmountOut))

	if poolInfo == nil {
		return nil, fmt.Errorf("pool info is nil")
	}

	// Validate and parse all necessary public keys
	ammProgramID, err := validatePublicKey(poolInfo.AmmProgramID)
	if err != nil {
		logger.Error("Invalid AmmProgramID", zap.Error(err))
		return nil, fmt.Errorf("invalid AmmProgramID: %w", err)
	}

	// Map of required accounts with their names
	requiredAccounts := map[string]string{
		"AmmID":                poolInfo.AmmID,
		"AmmAuthority":         poolInfo.AmmAuthority,
		"AmmOpenOrders":        poolInfo.AmmOpenOrders,
		"AmmTargetOrders":      poolInfo.AmmTargetOrders,
		"PoolCoinTokenAccount": poolInfo.PoolCoinTokenAccount,
		"PoolPcTokenAccount":   poolInfo.PoolPcTokenAccount,
		"SerumProgramID":       poolInfo.SerumProgramID,
		"SerumMarket":          poolInfo.SerumMarket,
		"SerumBids":            poolInfo.SerumBids,
		"SerumAsks":            poolInfo.SerumAsks,
		"SerumEventQueue":      poolInfo.SerumEventQueue,
		"SerumCoinVault":       poolInfo.SerumCoinVaultAccount,
		"SerumPcVault":         poolInfo.SerumPcVaultAccount,
		"SerumVaultSigner":     poolInfo.SerumVaultSigner,
	}

	accounts := make(map[string]solana.PublicKey)
	for name, address := range requiredAccounts {
		pubKey, err := validatePublicKey(address)
		if err != nil {
			logger.Error(fmt.Sprintf("Invalid %s", name),
				zap.String("address", address),
				zap.Error(err))
			return nil, fmt.Errorf("invalid %s: %w", name, err)
		}
		accounts[name] = pubKey
	}

	// Create the account meta slice in the correct order
	metas := solana.AccountMetaSlice{
		// User accounts
		{PublicKey: userWallet, IsSigner: true, IsWritable: false},
		{PublicKey: userSourceTokenAccount, IsSigner: false, IsWritable: true},
		{PublicKey: userDestinationTokenAccount, IsSigner: false, IsWritable: true},
		// Pool accounts
		{PublicKey: accounts["AmmID"], IsSigner: false, IsWritable: true},
		{PublicKey: accounts["AmmAuthority"], IsSigner: false, IsWritable: false},
		{PublicKey: accounts["AmmOpenOrders"], IsSigner: false, IsWritable: true},
		{PublicKey: accounts["AmmTargetOrders"], IsSigner: false, IsWritable: true},
		{PublicKey: accounts["PoolCoinTokenAccount"], IsSigner: false, IsWritable: true},
		{PublicKey: accounts["PoolPcTokenAccount"], IsSigner: false, IsWritable: true},
		// Serum accounts
		{PublicKey: accounts["SerumProgramID"], IsSigner: false, IsWritable: false},
		{PublicKey: accounts["SerumMarket"], IsSigner: false, IsWritable: true},
		{PublicKey: accounts["SerumBids"], IsSigner: false, IsWritable: true},
		{PublicKey: accounts["SerumAsks"], IsSigner: false, IsWritable: true},
		{PublicKey: accounts["SerumEventQueue"], IsSigner: false, IsWritable: true},
		{PublicKey: accounts["SerumCoinVault"], IsSigner: false, IsWritable: true},
		{PublicKey: accounts["SerumPcVault"], IsSigner: false, IsWritable: true},
		{PublicKey: accounts["SerumVaultSigner"], IsSigner: false, IsWritable: false},
		// System accounts
		{PublicKey: solana.TokenProgramID, IsSigner: false, IsWritable: false},
		{PublicKey: solana.SysVarClockPubkey, IsSigner: false, IsWritable: false},
		{PublicKey: solana.SysVarRentPubkey, IsSigner: false, IsWritable: false},
	}

	// Create swap instruction data
	instructionData := &SwapInstructionData{
		Instruction:  poolInfo.RaydiumSwapInstructionCode,
		AmountIn:     amountIn,
		MinAmountOut: minAmountOut,
	}

	// Serialize instruction data
	data, err := instructionData.Serialize()
	if err != nil {
		logger.Error("Failed to serialize instruction data",
			zap.Error(err),
			zap.Uint8("instruction", instructionData.Instruction),
			zap.Uint64("amount_in", instructionData.AmountIn),
			zap.Uint64("min_amount_out", instructionData.MinAmountOut))
		return nil, fmt.Errorf("failed to serialize instruction data: %w", err)
	}

	// Create the instruction
	instruction := solana.NewInstruction(ammProgramID, metas, data)

	logger.Debug("Created swap instruction",
		zap.Int("num_accounts", len(metas)),
		zap.Int("data_len", len(data)))

	return instruction, nil
}

// PrepareAndSendTransaction prepares and sends the swap transaction
func (r *DEX) PrepareAndSendTransaction(
	ctx context.Context,
	task *types.Task,
	userWallet *wallet.Wallet,
	logger *zap.Logger,
	swapInstruction solana.Instruction,
) error {
	recentBlockhash, err := r.client.GetRecentBlockhash(ctx)
	if err != nil {
		logger.Error("Failed to get recent blockhash", zap.Error(err))
		return fmt.Errorf("failed to get recent blockhash: %w", err)
	}

	// Create compute budget instruction if needed
	computeBudgetInst := computebudget.NewSetComputeUnitPriceInstruction(
		uint64(task.PriorityFee * 1e6), // Convert SOL to micro-lamports
	).Build()

	// Combine all instructions
	instructions := []solana.Instruction{
		computeBudgetInst,
		swapInstruction,
	}

	// Create the transaction
	tx, err := solana.NewTransaction(
		instructions,
		recentBlockhash,
		solana.TransactionPayer(userWallet.PublicKey),
	)
	if err != nil {
		logger.Error("Failed to create transaction", zap.Error(err))
		return fmt.Errorf("failed to create transaction: %w", err)
	}

	// Sign the transaction
	_, err = tx.Sign(func(key solana.PublicKey) *solana.PrivateKey {
		if key.Equals(userWallet.PublicKey) {
			return &userWallet.PrivateKey
		}
		return nil
	})
	if err != nil {
		logger.Error("Failed to sign transaction", zap.Error(err))
		return fmt.Errorf("failed to sign transaction: %w", err)
	}

	// Send the transaction
	signature, err := r.client.SendTransaction(ctx, tx)
	if err != nil {
		logger.Error("Failed to send transaction", zap.Error(err))
		return fmt.Errorf("failed to send transaction: %w", err)
	}

	logger.Info("Transaction sent successfully",
		zap.String("signature", signature.String()),
		zap.Float64("priority_fee_sol", task.PriorityFee))

	return nil
}

// validatePublicKey checks if a public key string is valid
func validatePublicKey(key string) (solana.PublicKey, error) {
	if key == "" {
		return solana.PublicKey{}, fmt.Errorf("empty public key")
	}

	pubKey, err := solana.PublicKeyFromBase58(key)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("invalid public key %s: %w", key, err)
	}

	return pubKey, nil
}

// Helper function to calculate minimum output considering slippage
func calculateMinimumOut(expectedOut float64, slippagePercent float64) uint64 {
	if expectedOut <= 0 {
		return 1 // Minimum safe value
	}

	// Consider slippage
	minOut := expectedOut * (1 - slippagePercent/100)

	// Convert to uint64 and check for minimum value
	result := uint64(math.Floor(minOut))
	if result == 0 {
		return 1
	}

	return result
}

// You may want to include other helper functions or adjust existing ones as needed
