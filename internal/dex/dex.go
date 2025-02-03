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
)

// DEX – единый интерфейс для работы с различными DEX.
type DEX interface {
	// GetName возвращает имя биржи.
	GetName() string
	// Execute выполняет операцию, заданную в task.
	Execute(ctx context.Context, task *Task) error
}

// pumpfunDEXAdapter – адаптер для Pump.fun, реализующий интерфейс DEX.
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
		d.logger.Info("Выполняется snipe на Pump.fun")
		// Для снайпа: task.Amount – количество (например, в лампортах), task.MinSolOutput – максимальная цена
		return d.inner.ExecuteSnipe(ctx, task.Amount, task.MinSolOutput)
	case OperationSell:
		d.logger.Info("Выполняется sell на Pump.fun")
		// Для продажи: task.Amount – количество, task.MinSolOutput – минимальный ожидаемый вывод SOL
		return d.inner.ExecuteSell(ctx, task.Amount, task.MinSolOutput)
	default:
		return fmt.Errorf("операция %s не поддерживается в Pump.fun", task.Operation)
	}
}

// raydiumDEXAdapter – адаптер для Raydium, реализующий интерфейс DEX.
type raydiumDEXAdapter struct {
	client *raydium.Client
	logger *zap.Logger
}

func (d *raydiumDEXAdapter) GetName() string {
	return "Raydium"
}

func (d *raydiumDEXAdapter) Execute(ctx context.Context, task *Task) error {
	switch task.Operation {
	// Здесь мы поддерживаем операцию swap, а также можем использовать её для быстрого buy (snipe)
	case OperationSwap, OperationSnipe:
		d.logger.Info("Выполняется swap/snipe на Raydium")
		// Формируем параметры для быстрого свопа (snipe) на Raydium.
		// Значения типа TokenMint, SourceMint и т.д. должны заполняться из конфигурации или получаться из контекста.
		snipeParams := &raydium.SnipeParams{
			TokenMint:           solana.PublicKey{}, // TODO: установить корректный mint токена
			SourceMint:          solana.MustPublicKeyFromBase58("SOURCE_MINT"),
			AmmAuthority:        solana.MustPublicKeyFromBase58("AMM_AUTHORITY"),
			BaseVault:           solana.MustPublicKeyFromBase58("BASE_VAULT"),
			QuoteVault:          solana.MustPublicKeyFromBase58("QUOTE_VAULT"),
			UserPublicKey:       solana.PublicKey{}, // TODO: установить публичный ключ пользователя
			PrivateKey:          nil,                // TODO: установить приватный ключ пользователя при необходимости
			UserSourceATA:       solana.MustPublicKeyFromBase58("USER_SOURCE_ATA"),
			UserDestATA:         solana.MustPublicKeyFromBase58("USER_DEST_ATA"),
			AmountInLamports:    task.Amount,
			MinOutLamports:      task.MinSolOutput,
			PriorityFeeLamports: 0,
		}
		_, err := d.client.Snipe(ctx, snipeParams)
		return err
	default:
		return fmt.Errorf("операция %s не поддерживается в Raydium", task.Operation)
	}
}

// GetDEXByName создаёт адаптер DEX по имени биржи.
// В качестве client для Pump.fun ожидается *solbc.Client, для Raydium – *raydium.Client.
func GetDEXByName(name string, client interface{}, logger *zap.Logger) (DEX, error) {
	if client == nil {
		return nil, fmt.Errorf("client не может быть nil")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger не может быть nil")
	}
	name = strings.ToLower(strings.TrimSpace(name))
	switch name {
	case "pump.fun":
		solClient, ok := client.(*solbc.Client)
		if !ok {
			return nil, fmt.Errorf("неверный тип client для Pump.fun; требуется *solbc.Client")
		}
		config := pumpfun.GetDefaultConfig(logger)
		pfDex, err := pumpfun.NewDEX(solClient, logger, config, config.MonitorInterval)
		if err != nil {
			return nil, fmt.Errorf("не удалось создать DEX для Pump.fun: %w", err)
		}
		return &pumpfunDEXAdapter{inner: pfDex, logger: logger}, nil
	case "raydium":
		raydClient, ok := client.(*raydium.Client)
		if !ok {
			return nil, fmt.Errorf("неверный тип client для Raydium; требуется *raydium.Client")
		}
		return &raydiumDEXAdapter{client: raydClient, logger: logger}, nil
	default:
		return nil, fmt.Errorf("биржа %s не поддерживается", name)
	}
}
