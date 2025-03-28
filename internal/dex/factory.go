// =============================
// File: internal/dex/factory.go
// =============================
package dex

import (
	"fmt"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc"
	"github.com/rovshanmuradov/solana-bot/internal/wallet"
	"go.uber.org/zap"
	"strings"
)

// GetDEXByName создаёт адаптер для DEX по имени биржи
func GetDEXByName(name string, client *solbc.Client, w *wallet.Wallet, logger *zap.Logger) (DEX, error) {
	if client == nil {
		return nil, fmt.Errorf("client cannot be nil")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}
	if w == nil {
		return nil, fmt.Errorf("wallet cannot be nil")
	}

	name = strings.ToLower(strings.TrimSpace(name))
	switch name {
	case "pump.fun":
		return &pumpfunDEXAdapter{
			baseDEXAdapter: baseDEXAdapter{
				client: client,
				wallet: w,
				logger: logger,
				name:   "Pump.fun",
			},
		}, nil

	case "pump.swap":
		return &pumpswapDEXAdapter{
			baseDEXAdapter: baseDEXAdapter{
				client: client,
				wallet: w,
				logger: logger,
				name:   "Pump.Swap",
			},
		}, nil

	default:
		return nil, fmt.Errorf("exchange %s is not supported", name)
	}
}
