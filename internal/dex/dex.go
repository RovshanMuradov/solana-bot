// =============================
// File: internal/dex/dex.go
// =============================
// internal/dex/dex.go
package dex

import (
	"context"
	"fmt"
	"strings"

	"github.com/gagliardetto/solana-go"
	"go.uber.org/zap"

	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc"
	"github.com/rovshanmuradov/solana-bot/internal/dex/pumpfun"
	"github.com/rovshanmuradov/solana-bot/internal/dex/raydium"
	"github.com/rovshanmuradov/solana-bot/internal/wallet"
)

// DEX is a unified interface for working with various DEXes.
type DEX interface {
	// GetName returns the name of the exchange.
	GetName() string
	// Execute performs the operation described in task.
	Execute(ctx context.Context, task *Task) error
}

// pumpfunDEXAdapter – adapter for Pump.fun implementing the DEX interface.
type pumpfunDEXAdapter struct {
	inner  *pumpfun.DEX
	logger *zap.Logger
}

func (d *pumpfunDEXAdapter) GetName() string {
	return "Pump.fun"
}

func (d *pumpfunDEXAdapter) Execute(ctx context.Context, task *Task) error {
	switch task.Operation {
	case OperationSnipe:
		d.logger.Info("Executing snipe on Pump.fun")
		return d.inner.ExecuteSnipe(ctx, task.Amount, task.MinSolOutput)
	case OperationSell:
		d.logger.Info("Executing sell on Pump.fun")
		return d.inner.ExecuteSell(ctx, task.Amount, task.MinSolOutput)
	default:
		return fmt.Errorf("operation %s is not supported on Pump.fun", task.Operation)
	}
}

// raydiumDEXAdapter – adapter for Raydium implementing the DEX interface.
type raydiumDEXAdapter struct {
	client *raydium.Client
	logger *zap.Logger
}

func (d *raydiumDEXAdapter) GetName() string {
	return "Raydium"
}

func (d *raydiumDEXAdapter) Execute(ctx context.Context, task *Task) error {
	switch task.Operation {
	case OperationSwap, OperationSnipe:
		d.logger.Info("Executing swap/snipe on Raydium")
		snipeParams := &raydium.SnipeParams{
			TokenMint:           solana.PublicKey{}, // fill with actual mint
			SourceMint:          solana.MustPublicKeyFromBase58("SOURCE_MINT"),
			AmmAuthority:        solana.MustPublicKeyFromBase58("AMM_AUTHORITY"),
			BaseVault:           solana.MustPublicKeyFromBase58("BASE_VAULT"),
			QuoteVault:          solana.MustPublicKeyFromBase58("QUOTE_VAULT"),
			UserPublicKey:       solana.PublicKey{}, // user's public key
			PrivateKey:          nil,                // user's private key if needed
			UserSourceATA:       solana.MustPublicKeyFromBase58("USER_SOURCE_ATA"),
			UserDestATA:         solana.MustPublicKeyFromBase58("USER_DEST_ATA"),
			AmountInLamports:    task.Amount,
			MinOutLamports:      task.MinSolOutput,
			PriorityFeeLamports: 0,
		}
		_, err := d.client.Snipe(ctx, snipeParams)
		return err
	default:
		return fmt.Errorf("operation %s is not supported on Raydium", task.Operation)
	}
}

// GetDEXByName creates a DEX adapter by exchange name.
// Обратите внимание: теперь функция принимает дополнительный параметр wallet.
func GetDEXByName(name string, client interface{}, w *wallet.Wallet, logger *zap.Logger) (DEX, error) {
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
		solClient, ok := client.(*solbc.Client)
		if !ok {
			return nil, fmt.Errorf("invalid client type for Pump.fun; *solbc.Client required")
		}
		// Получаем дефолтную конфигурацию для Pump.fun
		config := pumpfun.GetDefaultConfig(logger)
		// Передаём также экземпляр кошелька в конструктор NewDEX
		pfDex, err := pumpfun.NewDEX(solClient, w, logger, config, config.MonitorInterval)
		if err != nil {
			return nil, fmt.Errorf("could not create DEX for Pump.fun: %w", err)
		}
		return &pumpfunDEXAdapter{inner: pfDex, logger: logger}, nil

	case "raydium":
		raydClient, ok := client.(*raydium.Client)
		if !ok {
			return nil, fmt.Errorf("invalid client type for Raydium; *raydium.Client required")
		}
		return &raydiumDEXAdapter{client: raydClient, logger: logger}, nil

	default:
		return nil, fmt.Errorf("exchange %s is not supported", name)
	}
}
