// =============================================
// File: internal/dex/raydium/instructions.go
// =============================================
package raydium

import (
	"encoding/binary"
	"fmt"

	"github.com/gagliardetto/solana-go"
	computebudget "github.com/gagliardetto/solana-go/programs/compute-budget"
	"go.uber.org/zap"
)

// BuildSwapInstructions builds instructions for a Raydium swap.
func BuildSwapInstructions(params *SwapParams, logger *zap.Logger) ([]solana.Instruction, error) {
	var instructions []solana.Instruction

	if params.PriorityFeeLamports > 0 {
		ix1 := computebudget.NewSetComputeUnitLimitInstructionBuilder().
			SetUnits(params.ComputeUnits).
			Build()
		ix2 := computebudget.NewSetComputeUnitPriceInstructionBuilder().
			SetMicroLamports(params.PriorityFeeLamports).
			Build()
		instructions = append(instructions, ix1, ix2)
	}

	swapIx, err := buildRaydiumSwapIx(params, logger)
	if err != nil {
		return nil, err
	}
	instructions = append(instructions, swapIx)
	return instructions, nil
}

func buildRaydiumSwapIx(params *SwapParams, logger *zap.Logger) (solana.Instruction, error) {
	if params.PoolAddress.IsZero() || params.AmmAuthority.IsZero() {
		return nil, fmt.Errorf("missing pool addresses (Pool or AmmAuthority)")
	}

	// Example data layout: [u8(0x02), direction(1 byte), amountIn(uint64), minOut(uint64)]
	data := make([]byte, 1+1+8+8)
	data[0] = 0x02
	data[1] = byte(params.Direction)
	binary.LittleEndian.PutUint64(data[2:10], params.AmountInLamports)
	binary.LittleEndian.PutUint64(data[10:18], params.MinAmountOut)

	accounts := []*solana.AccountMeta{
		{PublicKey: params.PoolAddress, IsWritable: true, IsSigner: false},
		{PublicKey: params.AmmAuthority, IsWritable: false, IsSigner: false},
		{PublicKey: params.UserPublicKey, IsWritable: true, IsSigner: true},
		{PublicKey: params.UserSourceTokenAccount, IsWritable: true, IsSigner: false},
		{PublicKey: params.UserDestinationTokenAccount, IsWritable: true, IsSigner: false},
		{PublicKey: params.BaseVault, IsWritable: true, IsSigner: false},
		{PublicKey: params.QuoteVault, IsWritable: true, IsSigner: false},
		{PublicKey: TokenProgramID, IsWritable: false, IsSigner: false},
	}

	logger.Debug("Building Raydium swap instruction",
		zap.String("pool", params.PoolAddress.String()),
		zap.Uint64("amount_in", params.AmountInLamports),
		zap.Uint64("min_amount_out", params.MinAmountOut))

	return solana.NewInstruction(RaydiumProgramID, accounts, data), nil
}
