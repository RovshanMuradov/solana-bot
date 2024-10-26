// internal/dex/dex.go
package dex

import (
	"errors"
	"fmt"
	"strings"

	solanaClient "github.com/rovshanmuradov/solana-bot/internal/blockchain/solana"
	"github.com/rovshanmuradov/solana-bot/internal/dex/raydium"
	"github.com/rovshanmuradov/solana-bot/internal/types"
	"go.uber.org/zap"
)

func GetDEXByName(name string, client *solanaClient.Client, logger *zap.Logger) (types.DEX, error) {
	fmt.Printf("\n=== Getting DEX by name: %s ===\n", name)

	if logger == nil {
		fmt.Println("Logger is nil")
		return nil, errors.New("logger is nil")
	}

	name = strings.TrimSpace(name)
	if name == "" {
		fmt.Println("DEX name is empty")
		return nil, errors.New("DEX name cannot be empty")
	}

	fmt.Printf("Client nil? %v\n", client == nil)

	if client == nil {
		fmt.Println("Solana client is nil")
		return nil, errors.New("solana client cannot be nil")
	}

	switch strings.ToLower(name) {
	case strings.ToLower("Raydium"):
		fmt.Println("Creating Raydium DEX instance")

		if raydium.DefaultPoolConfig == nil {
			fmt.Println("Default pool config is nil")
			return nil, errors.New("default pool config is nil")
		}

		fmt.Printf("Pool config: %+v\n", raydium.DefaultPoolConfig)

		dex := raydium.NewDEX(client, logger, raydium.DefaultPoolConfig)
		if dex == nil {
			fmt.Println("Failed to create Raydium DEX instance")
			return nil, errors.New("failed to create Raydium DEX instance")
		}

		fmt.Printf("DEX created: %+v\n", dex)
		return dex, nil

	default:
		fmt.Printf("Unsupported DEX: %s\n", name)
		return nil, fmt.Errorf("unsupported DEX: %s", name)
	}
}
