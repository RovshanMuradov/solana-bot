// internal/dex/raydium/validation.go
package raydium

import (
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/rovshanmuradov/solana-bot/internal/types"
)

// ValidateTask проверяет корректность параметров задачи
func ValidateTask(task *types.Task) error {
	if task == nil {
		return fmt.Errorf("task cannot be nil")
	}

	if task.TaskName == "" {
		return fmt.Errorf("task name cannot be empty")
	}

	if task.SourceToken == "" {
		return fmt.Errorf("source token cannot be empty")
	}

	if task.TargetToken == "" {
		return fmt.Errorf("target token cannot be empty")
	}

	// Проверяем корректность адресов токенов
	if _, err := solana.PublicKeyFromBase58(task.SourceToken); err != nil {
		return fmt.Errorf("invalid source token address: %w", err)
	}

	if _, err := solana.PublicKeyFromBase58(task.TargetToken); err != nil {
		return fmt.Errorf("invalid target token address: %w", err)
	}

	if task.AmountIn <= 0 {
		return fmt.Errorf("amount in must be greater than 0")
	}

	// Удаляем проверку MinAmountOut, так как теперь оно может быть нулевым или пустым
	// if task.MinAmountOut <= 0 {
	//     return fmt.Errorf("min amount out must be greater than 0")
	// }

	// Проверяем конфигурацию проскальзывания, если она используется
	if task.SlippageConfig.Type != types.SlippageNone {
		if task.SlippageConfig.Type == types.SlippagePercent &&
			(task.SlippageConfig.Value <= 0 || task.SlippageConfig.Value > 100) {
			return fmt.Errorf("slippage percentage must be between 0 and 100")
		}
		if task.SlippageConfig.Type == types.SlippageFixed && task.SlippageConfig.Value < 0 {
			return fmt.Errorf("fixed slippage value cannot be negative")
		}
	}

	if task.SourceTokenDecimals <= 0 {
		return fmt.Errorf("source token decimals must be greater than 0")
	}

	if task.TargetTokenDecimals <= 0 {
		return fmt.Errorf("target token decimals must be greater than 0")
	}

	return nil
}

// ValidatePool проверяет корректность конфигурации пула
func ValidatePool(pool *Pool) error {
	if pool == nil {
		return fmt.Errorf("pool config cannot be nil")
	}

	// Проверяем все обязательные адреса
	addresses := map[string]string{
		"AmmProgramID":          pool.AmmProgramID,
		"AmmID":                 pool.AmmID,
		"AmmAuthority":          pool.AmmAuthority,
		"AmmOpenOrders":         pool.AmmOpenOrders,
		"AmmTargetOrders":       pool.AmmTargetOrders,
		"PoolCoinTokenAccount":  pool.PoolCoinTokenAccount,
		"PoolPcTokenAccount":    pool.PoolPcTokenAccount,
		"SerumProgramID":        pool.SerumProgramID,
		"SerumMarket":           pool.SerumMarket,
		"SerumBids":             pool.SerumBids,
		"SerumAsks":             pool.SerumAsks,
		"SerumEventQueue":       pool.SerumEventQueue,
		"SerumCoinVaultAccount": pool.SerumCoinVaultAccount,
		"SerumPcVaultAccount":   pool.SerumPcVaultAccount,
		"SerumVaultSigner":      pool.SerumVaultSigner,
	}

	for name, addr := range addresses {
		if addr == "" {
			return fmt.Errorf("%s cannot be empty", name)
		}

		if _, err := solana.PublicKeyFromBase58(addr); err != nil {
			return fmt.Errorf("invalid %s address: %w", name, err)
		}
	}

	return nil
}
