// internal/dex/raydium/raydium_bench_test.go
package raydium

import (
	"context"
	"testing"

	solanaGo "github.com/gagliardetto/solana-go"
	"go.uber.org/zap"

	"github.com/rovshanmuradov/solana-bot/internal/types"
	"github.com/rovshanmuradov/solana-bot/internal/wallet"
)

// BenchmarkPrepareSwapInstruction измеряет производительность подготовки инструкции свапа
func BenchmarkPrepareSwapInstruction(b *testing.B) {
	logger := zap.NewNop()
	dex := NewTestDEX(nil, nil)
	wallet := solanaGo.NewWallet()
	sourceToken := solanaGo.NewWallet().PublicKey()
	destToken := solanaGo.NewWallet().PublicKey()

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := dex.PrepareSwapInstruction(
			ctx,
			wallet.PublicKey(),
			sourceToken,
			destToken,
			1000000000,
			900000000,
			logger,
		)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCreateSwapInstruction измеряет производительность создания инструкции свапа
func BenchmarkCreateSwapInstruction(b *testing.B) {
	logger := zap.NewNop()

	dex := NewTestDEX(nil, nil)
	wallet := solanaGo.NewWallet()
	sourceToken := solanaGo.NewWallet().PublicKey()
	destToken := solanaGo.NewWallet().PublicKey()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := dex.CreateSwapInstruction(
			wallet.PublicKey(),
			sourceToken,
			destToken,
			1000000000,
			900000000,
			logger,
			TestPoolConfig,
		)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkExecuteSwap измеряет производительность выполнения свапа
func BenchmarkExecuteSwap(b *testing.B) {

	dex := NewTestDEX(nil, nil)
	testWallet := &wallet.Wallet{
		PrivateKey: solanaGo.NewWallet().PrivateKey,
		PublicKey:  solanaGo.NewWallet().PublicKey(),
	}

	task := &types.Task{
		AmountIn:                    1.0,
		MinAmountOut:                0.9,
		SourceTokenDecimals:         9,
		TargetTokenDecimals:         9,
		UserSourceTokenAccount:      solanaGo.NewWallet().PublicKey(),
		UserDestinationTokenAccount: solanaGo.NewWallet().PublicKey(),
		PriorityFee:                 0.000001,
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := dex.ExecuteSwap(ctx, task, testWallet)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSwapInstructionDataSerialization измеряет производительность сериализации
func BenchmarkSwapInstructionDataSerialization(b *testing.B) {
	data := SwapInstructionData{
		Instruction:  9,
		AmountIn:     1000000000,
		MinAmountOut: 900000000,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := data.Serialize()
		if err != nil {
			b.Fatal(err)
		}
	}
}
