// internal/dex/dex.go
package dex

import (
	"fmt"

	solanaClient "github.com/rovshanmuradov/solana-bot/internal/blockchain/solana"
	"github.com/rovshanmuradov/solana-bot/internal/dex/pumpfun"
	"github.com/rovshanmuradov/solana-bot/internal/dex/raydium"
	"github.com/rovshanmuradov/solana-bot/internal/types"
	"go.uber.org/zap"
)

func GetDEXByName(name string, client *solanaClient.Client, logger *zap.Logger) (types.DEX, error) {
	switch name {
	case "Raydium":
		return raydium.NewDEX(client, logger, raydium.DefaultPoolConfig), nil
	case "Pump.fun":
		return pumpfun.NewDEX(), nil
	default:
		return nil, fmt.Errorf("unsupported DEX: %s", name)
	}
}
