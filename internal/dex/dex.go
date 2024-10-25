// internal/dex/dex.go
package dex

import (
	"errors"
	"fmt"

	solanaClient "github.com/rovshanmuradov/solana-bot/internal/blockchain/solana"
	"github.com/rovshanmuradov/solana-bot/internal/dex/pumpfun"
	"github.com/rovshanmuradov/solana-bot/internal/dex/raydium"
	"github.com/rovshanmuradov/solana-bot/internal/types"
	"go.uber.org/zap"
)

func GetDEXByName(name string, client *solanaClient.Client, logger *zap.Logger) (types.DEX, error) {
	logger.Debug("Getting DEX module",
		zap.String("dex_name", name),
		zap.Bool("has_client", client != nil))

	if name == "" {
		return nil, errors.New("DEX name cannot be empty")
	}

	switch name {
	case "Raydium":
		return raydium.NewDEX(client, logger, raydium.DefaultPoolConfig), nil
	case "Pump.fun":
		return pumpfun.NewDEX(), nil
	default:
		return nil, fmt.Errorf("unsupported DEX: %s", name)
	}
}
