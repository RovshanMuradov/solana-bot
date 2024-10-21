// internal/dex/dex.go
// interface dex
package dex

import (
	"fmt"

	"github.com/rovshanmuradov/solana-bot/internal/dex/pumpfun"
	"github.com/rovshanmuradov/solana-bot/internal/dex/raydium"
	"github.com/rovshanmuradov/solana-bot/internal/types"
)

func GetDEXByName(name string) (types.DEX, error) {
	switch name {
	case "Raydium":
		return raydium.NewRaydiumDEX(), nil
	case "Pump.fun":
		return pumpfun.NewPumpFunDEX(), nil
	default:
		return nil, fmt.Errorf("unsupported DEX: %s", name)
	}
}
