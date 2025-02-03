// internal/dex/raydium/instructions.go
package raydium

import (
	"encoding/binary"
	"fmt"

	"github.com/gagliardetto/solana-go"
	computebudget "github.com/gagliardetto/solana-go/programs/compute-budget"
	"go.uber.org/zap"
)

// BuildSwapInstructions builds the instructions needed for a Raydium swap.
func BuildSwapInstructions(params *SwapParams, logger *zap.Logger) ([]solana.Instruction, error) {
	var instructions []solana.Instruction

	// 1. Optionally add compute budget instructions if PriorityFeeLamports > 0
	if params.PriorityFeeLamports > 0 {
		ix1 := computebudget.NewSetComputeUnitLimitInstructionBuilder().
			SetUnits(params.ComputeUnits).
			Build()
		ix2 := computebudget.NewSetComputeUnitPriceInstructionBuilder().
			SetMicroLamports(params.PriorityFeeLamports).
			Build()
		instructions = append(instructions, ix1, ix2)
	}

	// 2. Prepare the main Raydium swap instruction
	swapIx, err := buildRaydiumSwapIx(params, logger)
	if err != nil {
		return nil, err
	}
	instructions = append(instructions, swapIx)

	return instructions, nil
}

// buildRaydiumSwapIx is an internal function that encodes the Raydium swap logic.
func buildRaydiumSwapIx(params *SwapParams, logger *zap.Logger) (solana.Instruction, error) {
	if params.PoolAddress.IsZero() || params.AmmAuthority.IsZero() {
		return nil, fmt.Errorf("missing pool addresses (Pool or AmmAuthority)")
	}

	// Example layout for data: [ u8(0x02) = swap, direction(1byte), amountIn(uint64), minOut(uint64) ]
	data := make([]byte, 1+1+8+8)
	data[0] = 0x02                   // Suppose 0x02 is "swap" instruction code in Raydium
	data[1] = byte(params.Direction) // 0 or 1
	binary.LittleEndian.PutUint64(data[2:10], params.AmountInLamports)
	binary.LittleEndian.PutUint64(data[10:18], params.MinAmountOut)

	accounts := []*solana.AccountMeta{
		// Main pool account
		{PublicKey: params.PoolAddress, IsWritable: true, IsSigner: false},
		// Amm Authority
		{PublicKey: params.AmmAuthority, IsWritable: false, IsSigner: false},
		// Payer signature
		{PublicKey: params.UserPublicKey, IsWritable: true, IsSigner: true},
		// Source user token account
		{PublicKey: params.UserSourceTokenAccount, IsWritable: true, IsSigner: false},
		// Destination user token account
		{PublicKey: params.UserDestinationTokenAccount, IsWritable: true, IsSigner: false},
		// Additional Raydium vaults, token program, etc. (simplified)
		{PublicKey: params.BaseVault, IsWritable: true, IsSigner: false},
		{PublicKey: params.QuoteVault, IsWritable: true, IsSigner: false},
		{PublicKey: TokenProgramID, IsWritable: false, IsSigner: false},
	}

	logger.Debug("Building Raydium swap instruction",
		zap.String("pool", params.PoolAddress.String()),
		zap.Uint64("amount_in", params.AmountInLamports),
		zap.Uint64("min_amount_out", params.MinAmountOut),
	)

	return solana.NewInstruction(RaydiumProgramID, accounts, data), nil
}
