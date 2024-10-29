package dex

import (
	"fmt"
	"strings"

	"github.com/rovshanmuradov/solana-bot/internal/blockchain"
	"github.com/rovshanmuradov/solana-bot/internal/dex/pumpfun"
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
		return initializePumpFunDEX(client, logger)
	default:
		logger.Error("Unsupported DEX requested", zap.String("name", name))
		return nil, fmt.Errorf("unsupported DEX: %s", name)
	}
}

// initializeRaydiumDEX инициализирует Raydium DEX
func initializeRaydiumDEX(client blockchain.Client, logger *zap.Logger) (types.DEX, error) {
	logger.Debug("Initializing Raydium DEX")

	if raydium.DefaultPoolConfig == nil {
		logger.Error("Default pool configuration is missing")
		return nil, fmt.Errorf("raydium default pool config is nil")
	}

	logger.Debug("Creating Raydium DEX instance",
		zap.String("pool_id", raydium.DefaultPoolConfig.AmmID),
		zap.String("program_id", raydium.DefaultPoolConfig.AmmProgramID))

	dex := raydium.NewDEX(client, logger, raydium.DefaultPoolConfig)
	if dex == nil {
		logger.Error("Failed to create Raydium DEX instance")
		return nil, fmt.Errorf("failed to create Raydium DEX instance")
	}

	logger.Info("Raydium DEX initialized successfully")
	return dex, nil
}

// initializePumpFunDEX инициализирует Pump.fun DEX
func initializePumpFunDEX(_ blockchain.Client, logger *zap.Logger) (types.DEX, error) {
	logger.Debug("Initializing Pump.fun DEX")

	// Создаем новый экземпляр Pump.fun DEX
	dex := pumpfun.NewDEX()
	if dex == nil {
		logger.Error("Failed to create Pump.fun DEX instance")
		return nil, fmt.Errorf("failed to create Pump.fun DEX instance")
	}

	logger.Info("Pump.fun DEX initialized successfully")
	return dex, nil
}
