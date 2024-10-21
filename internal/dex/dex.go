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
		poolInfo := &raydium.RaydiumPoolInfo{
			// Инициализируйте поля реальными значениями
		}
		return raydium.NewRaydiumDEX(client, logger, poolInfo), nil
	case "Pump.fun":
		return pumpfun.NewPumpFunDEX(), nil
	default:
		return nil, fmt.Errorf("unsupported DEX: %s", name)
	}
}
