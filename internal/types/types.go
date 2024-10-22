// internal/types/types.go
package types

import (
	"context"

	"github.com/gagliardetto/solana-go"
	"go.uber.org/zap"
)

type Task struct {
	TaskName                    string
	Module                      string
	Workers                     int
	WalletName                  string
	Delta                       int
	PriorityFee                 float64
	AMMID                       string
	SourceToken                 string
	TargetToken                 string
	AmountIn                    float64
	MinAmountOut                float64
	AutosellPercent             float64
	AutosellDelay               int
	AutosellAmount              float64
	TransactionDelay            int
	AutosellPriorityFee         float64
	UserSourceTokenAccount      solana.PublicKey
	UserDestinationTokenAccount solana.PublicKey
	SourceTokenDecimals         int
	TargetTokenDecimals         int
	DEXName                     string
}

type DEX interface {
	Name() string
	PrepareSwapInstruction(
		ctx context.Context,
		wallet solana.PublicKey,
		sourceToken solana.PublicKey,
		destinationToken solana.PublicKey,
		amountIn uint64,
		minAmountOut uint64,
		logger *zap.Logger,
	) (solana.Instruction, error)
}

type Blockchain interface {
	Name() string
	SendTransaction(ctx context.Context, tx interface{}) (string, error)
}
