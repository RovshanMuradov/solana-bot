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
			// Заполните поля актуальными данными
			AmmProgramID:               "YourAmmProgramID",
			AmmID:                      "YourAmmID",
			AmmAuthority:               "YourAmmAuthority",
			AmmOpenOrders:              "YourAmmOpenOrders",
			AmmTargetOrders:            "YourAmmTargetOrders",
			PoolCoinTokenAccount:       "YourPoolCoinTokenAccount",
			PoolPcTokenAccount:         "YourPoolPcTokenAccount",
			SerumProgramID:             "YourSerumProgramID",
			SerumMarket:                "YourSerumMarket",
			SerumBids:                  "YourSerumBids",
			SerumAsks:                  "YourSerumAsks",
			SerumEventQueue:            "YourSerumEventQueue",
			SerumCoinVaultAccount:      "YourSerumCoinVaultAccount",
			SerumPcVaultAccount:        "YourSerumPcVaultAccount",
			SerumVaultSigner:           "YourSerumVaultSigner",
			RaydiumSwapInstructionCode: 123, // Замените на актуальный код инструкции свапа
		}
		return raydium.NewRaydiumDEX(client, logger, poolInfo), nil
	case "Pump.fun":
		return pumpfun.NewPumpFunDEX(), nil
	default:
		return nil, fmt.Errorf("unsupported DEX: %s", name)
	}
}
