// internal/dex/dex.go
package dex

import (
	"fmt"
	"strings"
	"time"

	"github.com/rovshanmuradov/solana-bot/internal/blockchain"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc"
	"github.com/rovshanmuradov/solana-bot/internal/dex/raydium"
	"github.com/rovshanmuradov/solana-bot/internal/types"
	"go.uber.org/zap"
)

// GetDEXByName возвращает имплементацию DEX по имени
func GetDEXByName(name string, client blockchain.Client, logger *zap.Logger) (types.DEX, error) {
	if client == nil {
		return nil, fmt.Errorf("client cannot be nil")
	}

	if logger == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}

	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return nil, fmt.Errorf("DEX name cannot be empty")
	}

	logger = logger.With(zap.String("dex_name", name))
	logger.Info("Initializing DEX instance")

	switch name {
	case "raydium":
		return initializeRaydiumDEX(client, logger)
	case "pump.fun":
		return nil /*initializePumpFunDEX(client, logger)*/, nil
	default:
		logger.Error("Unsupported DEX requested", zap.String("name", name))
		return nil, fmt.Errorf("unsupported DEX: %s", name)
	}
}

// initializeRaydiumDEX инициализирует Raydium DEX
func initializeRaydiumDEX(client blockchain.Client, logger *zap.Logger) (types.DEX, error) {
	solClient, ok := client.(*solbc.Client)
	if !ok {
		return nil, fmt.Errorf("invalid client type")
	}

	// Получаем RPC endpoint
	endpoint := solClient.GetRPCEndpoint()
	if endpoint == "" {
		return nil, fmt.Errorf("empty RPC endpoint")
	}

	// Получаем приватный ключ
	walletKey, err := solClient.GetWalletKey()
	if err != nil {
		return nil, fmt.Errorf("failed to get wallet key: %w", err)
	}

	// Создаем Raydium клиент
	raydiumClient, err := raydium.NewRaydiumClient(endpoint, walletKey, logger.Named("raydium"))
	if err != nil {
		return nil, fmt.Errorf("failed to create Raydium client: %w", err)
	}

	// Создаем конфигурацию
	config := &raydium.SniperConfig{
		MaxSlippageBps:   500,        // 5%
		MinAmountSOL:     100000,     // 0.0001 SOL
		MaxAmountSOL:     1000000000, // 1 SOL
		PriorityFee:      1000,
		WaitConfirmation: true,
		MonitorInterval:  time.Second,
		MaxRetries:       3,
	}

	return &raydiumDEX{
		client: raydiumClient,
		logger: logger,
		config: config,
	}, nil
}

// // initializePumpFunDEX инициализирует Pump.fun DEX
// func initializePumpFunDEX(_ blockchain.Client, logger *zap.Logger) (types.DEX, error) {
// 	logger.Debug("Initializing Pump.fun DEX")

// 	// Создаем новый экземпляр Pump.fun DEX
// 	dex := pumpfun.NewDEX()
// 	if dex == nil {
// 		logger.Error("Failed to create Pump.fun DEX instance")
// 		return nil, fmt.Errorf("failed to create Pump.fun DEX instance")
// 	}

// 	logger.Info("Pump.fun DEX initialized successfully")
// 	return dex, nil
// }
