// internal/dex/pumpfun/graduate.go
package pumpfun

import (
	"context"
	"fmt"

	"github.com/gagliardetto/solana-go"
	solanarpc "github.com/gagliardetto/solana-go/rpc"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc"
	"go.uber.org/zap"
)

// GraduateParams содержит параметры для транзакции graduate.
type GraduateParams struct {
	// Mint токена, созданного через Pump.fun.
	TokenMint solana.PublicKey
	// Bonding curve аккаунт.
	BondingCurveAccount solana.PublicKey
	// Дополнительные параметры (например, ликвидность, fee и т.д.).
	ExtraData []byte
}

// GraduateToken выполняет транзакцию graduate, переводя токен на Raydium.
func GraduateToken(ctx context.Context, client *solbc.Client, logger *zap.Logger, params *GraduateParams) (solana.Signature, error) {
	logger.Info("Initiating graduate process", zap.String("token_mint", params.TokenMint.String()))

	graduateIx, err := BuildGraduateInstruction(params)
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

// BuildGraduateInstruction собирает инструкцию для graduate.
func BuildGraduateInstruction(params *GraduateParams) (solana.Instruction, error) {
	discriminator := byte(0x22)
	name := "PumpToken"
	symbol := "PUMP"
	uri := "https://ipfs.io/ipfs/QmExample"

	data := BuildGraduateInstructionData(discriminator, name, symbol, uri, params.ExtraData)

	accounts := []solana.AccountMeta{
		*solana.Meta(params.TokenMint).WRITE(),
		*solana.Meta(params.BondingCurveAccount).WRITE(),
		// Добавьте остальные необходимые аккаунты согласно спецификации.
	}

	// Используйте корректный programID (замените placeholder при необходимости).
	programID := params.TokenMint

	return solana.NewInstruction(programID, accounts, data), nil
}

// BuildGraduateInstructionData сериализует данные для graduate-инструкции.
func BuildGraduateInstructionData(discriminator byte, name, symbol, uri string, extra []byte) []byte {
	data := []byte{discriminator}
	data = append(data, []byte(name)...)
	data = append(data, 0) // разделитель
	data = append(data, []byte(symbol)...)
	data = append(data, 0)
	data = append(data, []byte(uri)...)
	if len(extra) > 0 {
		data = append(data, extra...)
	}
	return data
}
