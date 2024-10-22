// internal/dex/pumpfun/pumpfun.go
package pumpfun

import (
	"context"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/rovshanmuradov/solana-bot/internal/types"
	"github.com/rovshanmuradov/solana-bot/internal/wallet"
	"go.uber.org/zap"
)

// DEX представляет реализацию Pump.fun DEX (временная заглушка)
type DEX struct {
	// Будет реализовано позже
}

func NewDEX() *DEX {
	return &DEX{}
}

func (p *DEX) Name() string {
	return "Pump.fun"
}

func (p *DEX) PrepareSwapInstruction(
	_ context.Context,
	_ solana.PublicKey,
	_ solana.PublicKey,
	_ solana.PublicKey,
	_ uint64,
	_ uint64,
	_ *zap.Logger,
) (solana.Instruction, error) {
	return nil, fmt.Errorf("pump.fun DEX implementation not ready")
}

func (p *DEX) ExecuteSwap(
	_ context.Context,
	_ *types.Task,
	_ *wallet.Wallet,
) error {
	return fmt.Errorf("pump.fun DEX implementation not ready")
}
