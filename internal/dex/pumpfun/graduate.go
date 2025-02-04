// ==============================================
// File: internal/dex/pumpfun/graduate.go
// ==============================================
package pumpfun

import (
	"context"
	"fmt"

	"github.com/gagliardetto/solana-go"
	solanarpc "github.com/gagliardetto/solana-go/rpc"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc"
	"go.uber.org/zap"
)

// GraduateParams holds transaction params for "graduate".
type GraduateParams struct {
	TokenMint           solana.PublicKey
	BondingCurveAccount solana.PublicKey
	ExtraData           []byte
}

// GraduateToken sends a transaction to move from Pump.fun to Raydium.
func GraduateToken(ctx context.Context, client *solbc.Client, logger *zap.Logger, params *GraduateParams, programID solana.PublicKey) (solana.Signature, error) {
	logger.Info("Initiating graduate process", zap.String("token_mint", params.TokenMint.String()))
	graduateIx, err := BuildGraduateInstruction(params, programID)
	if err != nil {
		return solana.Signature{}, fmt.Errorf("failed to build graduate instruction: %w", err)
	}

	recentBlockhash, err := client.GetRecentBlockhash(ctx)
	if err != nil {
		return solana.Signature{}, fmt.Errorf("failed to get recent blockhash: %w", err)
	}

	tx, err := solana.NewTransaction(
		[]solana.Instruction{graduateIx},
		recentBlockhash,
		solana.TransactionPayer(client.GetWalletPublicKey()),
	)
	if err != nil {
		return solana.Signature{}, fmt.Errorf("failed to create graduate transaction: %w", err)
	}

	if err := client.SignTransaction(tx); err != nil {
		return solana.Signature{}, fmt.Errorf("failed to sign graduate transaction: %w", err)
	}

	sig, err := client.SendTransaction(ctx, tx)
	if err != nil {
		return solana.Signature{}, fmt.Errorf("failed to send graduate transaction: %w", err)
	}

	logger.Info("Graduate transaction sent", zap.String("signature", sig.String()))

	if err := client.WaitForTransactionConfirmation(ctx, sig, solanarpc.CommitmentFinalized); err != nil {
		logger.Error("Graduate transaction confirmation failed", zap.Error(err))
	}

	return sig, nil
}

// BuildGraduateInstruction creates the instruction for graduate.
func BuildGraduateInstruction(params *GraduateParams, programID solana.PublicKey) (solana.Instruction, error) {
	discriminator := byte(0x22)
	name := "PumpToken"
	symbol := "PUMP"
	uri := "https://ipfs.io/ipfs/QmExample"

	data := BuildGraduateInstructionData(discriminator, name, symbol, uri, params.ExtraData)

	accounts := []*solana.AccountMeta{
		solana.Meta(params.TokenMint).WRITE(),
		solana.Meta(params.BondingCurveAccount).WRITE(),
	}

	return solana.NewInstruction(programID, accounts, data), nil
}

// BuildGraduateInstructionData serializes graduate instruction data.
func BuildGraduateInstructionData(discriminator byte, name, symbol, uri string, extra []byte) []byte {
	data := []byte{discriminator}
	data = append(data, []byte(name)...)
	data = append(data, 0)
	data = append(data, []byte(symbol)...)
	data = append(data, 0)
	data = append(data, []byte(uri)...)
	if len(extra) > 0 {
		data = append(data, extra...)
	}
	return data
}
